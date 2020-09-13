package session

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/internal/traffic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ConnectionManager is an interface required by the session to open and release
// connections to the relays.
type ConnectionManager interface {
	Acquire(mino.Address) (grpc.ClientConnInterface, error)
	Release(mino.Address)
}

// Session is an interface for a stream session that allows to send messages to
// the parent and relays, while receiving the ones for the local address.
type Session interface {
	mino.Sender
	mino.Receiver

	Listen()
}

// Relay is the interface of the relays spawn by the session when trying to
// contact a child node.
type Relay interface {
	Context() context.Context
	Recv() (router.Packet, error)
	Send(router.Packet) error
	Close() error
}

type session struct {
	sync.Mutex
	sync.WaitGroup

	logger  zerolog.Logger
	md      metadata.MD
	me      mino.Address
	gateway Relay
	errs    chan error
	table   router.RoutingTable
	pktFac  router.PacketFactory
	msgFac  serde.Factory
	context serde.Context
	queue   Queue
	relays  map[mino.Address]Relay
	connMgr ConnectionManager
	traffic *traffic.Traffic
}

// NewSession creates a new session for the provided stream.
func NewSession(
	md metadata.MD,
	stream PacketStream,
	me mino.Address,
	table router.RoutingTable,
	msgFac serde.Factory,
	pktFac router.PacketFactory,
	ctx serde.Context,
	connMgr ConnectionManager,
) Session {
	sess := &session{
		logger:  dela.Logger.With().Str("addr", me.String()).Logger(),
		md:      md,
		me:      me,
		errs:    make(chan error, 1),
		table:   table,
		msgFac:  msgFac,
		pktFac:  pktFac,
		context: ctx,
		queue:   newNonBlockingQueue(),
		relays:  make(map[mino.Address]Relay),
		connMgr: connMgr,
	}

	switch os.Getenv(traffic.EnvVariable) {
	case "log":
		sess.traffic = traffic.NewTraffic(me, ioutil.Discard)
	case "print":
		sess.traffic = traffic.NewTraffic(me, os.Stdout)
	}

	gateway := newRelay(stream, pktFac, ctx)
	gateway.traffic = sess.traffic

	sess.gateway = gateway

	return sess
}

// Listen implements session.Session. It listens for incoming packets from the
// parent stream and closes the relays when it is done.
func (s *session) Listen() {
	defer func() {
		s.close()
		close(s.errs)
	}()

	for {
		packet, err := s.gateway.Recv()
		if err == io.EOF {
			return
		}
		status, ok := status.FromError(err)
		if ok && status.Code() == codes.Canceled {
			return
		}
		if err != nil {
			s.errs <- xerrors.Errorf("failed to receive: %v", err)
			return
		}

		err = s.sendPacket(s.gateway.Context(), packet)
		if err != nil {
			s.logger.Err(err).Msg("failed to send to dispatched relays")
		}
	}
}

// Send implements mino.Sender. It sends the message to the provided addresses
// through the relays or the parent.
func (s *session) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, 1)
	defer close(errs)

	data, err := msg.Serialize(s.context)
	if err != nil {
		errs <- xerrors.Errorf("failed to serialize msg: %v", err)
		return errs
	}

	packet := s.table.Make(s.me, addrs, data)

	err = s.sendPacket(s.gateway.Context(), packet)
	if err != nil {
		errs <- xerrors.Errorf("packet: %v", err)
		return errs
	}

	return errs
}

// Recv implements mino.Receiver. It waits for a message to arrive and returns
// it, or returns an error if something wrong happens. The context can cancel
// the blocking call.
func (s *session) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case err := <-s.errs:
		if err != nil {
			return nil, nil, xerrors.Errorf("stream closed unexpectedly: %v", err)
		}

		return nil, nil, io.EOF
	case packet := <-s.queue.Channel():
		msg, err := s.msgFac.Deserialize(s.context, packet.GetMessage())
		if err != nil {
			return nil, nil, xerrors.Errorf("message: %v", err)
		}

		return packet.GetSource(), msg, nil
	}
}

func (s *session) close() {
	s.Lock()
	defer s.Unlock()

	for to, relay := range s.relays {
		err := relay.Close()

		s.traffic.LogRelayClosed(to)
		s.logger.Trace().Err(err).Msg("relay closed")

		// Let the manager know it can close the connection if necessary.
		s.connMgr.Release(to)
	}

	s.Wait()
}

func (s *session) sendPacket(ctx context.Context, p router.Packet) error {
	me := p.Slice(s.me)
	if me != nil {
		// TODO: check error after merging PR #104
		s.queue.Push(me)
	}

	if len(p.GetDestination()) == 0 {
		return nil
	}

	routes, err := s.table.Forward(p)
	if err != nil {
		return xerrors.Errorf("routing table: %v", err)
	}

	for addr, packet := range routes {
		var relay Relay
		if addr == nil {
			relay = s.gateway
		} else {
			relay, err = s.setupRelay(ctx, addr)
			if err != nil {
				return xerrors.Errorf("relay aborted: %v", err)
			}
		}

		err := relay.Send(packet)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *session) setupRelay(ctx context.Context, addr mino.Address) (Relay, error) {
	s.Lock()
	defer s.Unlock()

	relay, initiated := s.relays[addr]

	if initiated {
		return relay, nil
	}

	// 1. Acquire a connection to the distant peer.
	conn, err := s.connMgr.Acquire(addr)
	if err != nil {
		return nil, xerrors.Errorf("failed to dial: %v", err)
	}

	cl := ptypes.NewOverlayClient(conn)
	ctx = metadata.NewOutgoingContext(ctx, s.md)

	stream, err := cl.Stream(ctx, grpc.WaitForReady(false))
	if err != nil {
		return nil, xerrors.Errorf("client: %v", err)
	}

	// 2. Send the first message which is the router handshake.
	hs, err := s.table.Prelude(addr).Serialize(s.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize handshake: %v", err)
	}

	err = stream.Send(&ptypes.Packet{Serialized: hs})
	if err != nil {
		return nil, xerrors.Errorf("failed to send handshake: %v", err)
	}

	// 3. Create and run the relay to respond to incoming packets.
	newRelay := newRelay(stream, s.pktFac, s.context)
	newRelay.gw = addr
	newRelay.traffic = s.traffic

	s.relays[addr] = newRelay
	s.Add(1)

	go func() {
		defer s.Done()

		for {
			pkt, err := newRelay.Recv()
			if err == io.EOF {
				return
			}
			if status.Code(err) == codes.Canceled {
				return
			}
			if err != nil {
				s.logger.Err(err).Msg("relay failed")
				return
			}

			err = s.sendPacket(ctx, pkt)
			if err != nil {
				s.logger.Err(err).Msg("relay failed to send a packet")
				return
			}
		}
	}()

	s.traffic.LogRelay(addr)
	s.logger.Debug().
		Str("to", addr.String()).
		Msg("relay opened")

	return newRelay, nil
}

// PacketStream is a gRPC stream to send and receive protobuf packets.
type PacketStream interface {
	Context() context.Context
	Send(*ptypes.Packet) error
	Recv() (*ptypes.Packet, error)
}

// Relay is a relay open to a distant peer in a session. It offers functions to
// send and receive messages.
type relay struct {
	sync.Mutex
	gw        mino.Address
	stream    PacketStream
	packetFac router.PacketFactory
	context   serde.Context
	traffic   *traffic.Traffic
}

func newRelay(stream PacketStream, fac router.PacketFactory, ctx serde.Context) *relay {
	r := &relay{
		stream:    stream,
		packetFac: fac,
		context:   ctx,
	}

	return r
}

// Context implements session.Relay. It returns the context of the stream.
func (r *relay) Context() context.Context {
	return r.stream.Context()
}

// Recv implements session.Relay. It waits for a message from the distant peer
// and returns it.
func (r *relay) Recv() (router.Packet, error) {
	proto, err := r.stream.Recv()
	if err != nil {
		// The session will parse the error to detect specific events of the
		// stream, therefore no wrap is done here.
		return nil, err
	}

	packet, err := r.packetFac.PacketOf(r.context, proto.Serialized)
	if err != nil {
		return nil, xerrors.Errorf("packet: %v", err)
	}

	r.traffic.LogRecv(r.stream.Context(), r.gw, packet)

	return packet, nil
}

// Send implements session.Relay. It sends the message to the distant peer.
func (r *relay) Send(p router.Packet) error {
	data, err := p.Serialize(r.context)
	if err != nil {
		return xerrors.Errorf("failed to serialize: %v", err)
	}

	r.Lock()
	defer r.Unlock()

	err = r.stream.Send(&ptypes.Packet{Serialized: data})
	if err != nil {
		return xerrors.Errorf("stream failed: %v", err)
	}

	r.traffic.LogSend(r.stream.Context(), r.gw, p)

	return nil
}

// Close implements session.Relay. It closes the stream.
func (r *relay) Close() error {
	stream, ok := r.stream.(ptypes.Overlay_StreamClient)
	if ok {
		err := stream.CloseSend()
		if err != nil {
			return xerrors.Errorf("failed to close stream: %v", err)
		}
	}

	return nil
}
