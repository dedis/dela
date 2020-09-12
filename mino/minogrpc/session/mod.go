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
	Len() int
	Acquire(mino.Address) (grpc.ClientConnInterface, error)
	Release(mino.Address)
}

// Session is an interface for a stream session that allows to send messages to
// the parent and relays, while receiving the ones for the local address.
type Session interface {
	mino.Sender
	mino.Receiver

	Listen(stream PacketStream)

	RecvPacket(from mino.Address, p *ptypes.Packet) (*ptypes.Ack, error)
}

// Relay is the interface of the relays spawn by the session when trying to
// contact a child node.
type Relay interface {
	Distant() mino.Address
	Stream() PacketStream
	Send(ctx context.Context, p router.Packet) (*ptypes.Ack, error)
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
	gw Relay,
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
		gateway: gw,
	}

	switch os.Getenv(traffic.EnvVariable) {
	case "log":
		sess.traffic = traffic.NewTraffic(me, ioutil.Discard)
	case "print":
		sess.traffic = traffic.NewTraffic(me, os.Stdout)
	}

	return sess
}

// Listen implements session.Session. It listens for incoming packets from the
// parent stream and closes the relays when it is done.
func (s *session) Listen(stream PacketStream) {
	defer func() {
		s.close()

		s.logger.Trace().Str("addr", s.me.String()).Msg("session has been closed")
	}()

	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if status.Code(err) == codes.Canceled {
			return
		}
		if err != nil {
			s.errs <- xerrors.Errorf("failed to receive: %v", err)
			return
		}
	}
}

func (s *session) RecvPacket(from mino.Address, p *ptypes.Packet) (*ptypes.Ack, error) {
	pkt, err := s.pktFac.PacketOf(s.context, p.GetSerialized())
	if err != nil {
		return nil, err
	}

	s.traffic.LogRecv(s.gateway.Stream().Context(), from, pkt)

	ack := &ptypes.Ack{}
	s.sendPacket(s.gateway.Stream().Context(), pkt, func(err error) {
		ack.Errors = append(ack.Errors, err.Error())
	})

	return ack, nil
}

// Send implements mino.Sender. It sends the message to the provided addresses
// through the relays or the parent.
func (s *session) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, len(addrs)+1)

	go func() {
		defer close(errs)

		data, err := msg.Serialize(s.context)
		if err != nil {
			errs <- xerrors.Errorf("failed to serialize msg: %v", err)
			return
		}

		packet := s.table.Make(s.me, addrs, data)

		s.sendPacket(s.gateway.Stream().Context(), packet, func(err error) {
			errs <- err
		})
	}()

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
	s.gateway.Close()
	close(s.errs)

	// Lock must be released to let the relays close themselves and clean the
	// map.
	s.Wait()
}

func (s *session) sendPacket(ctx context.Context, p router.Packet, fn func(error)) {
	me := p.Slice(s.me)
	if me != nil {
		s.queue.Push(me)
	}

	if len(p.GetDestination()) == 0 {
		return
	}

	routes, voids := s.table.Forward(p)
	for addr, err := range voids {
		fn(xerrors.Errorf("no route to %v: %v", addr, err))
	}

	wg := sync.WaitGroup{}
	wg.Add(len(routes))

	for addr, packet := range routes {
		go s.sendTo(ctx, addr, packet, fn, &wg)
	}

	wg.Wait()
}

func (s *session) sendTo(ctx context.Context, to mino.Address, pkt router.Packet, fn func(error), wg *sync.WaitGroup) {
	defer wg.Done()

	var relay Relay
	var err error

	if to == nil {
		relay = s.gateway
	} else {
		relay, err = s.setupRelay(ctx, to)
		if err != nil {
			s.logger.Warn().
				Err(err).
				Str("to", to.String()).
				Msg("failed to setup relay")

			// Try to open a different relay.
			s.onFailure(ctx, to, pkt, fn)

			return
		}
	}

	s.traffic.LogSend(ctx, relay.Distant(), pkt)

	ack, err := relay.Send(ctx, pkt)
	if to == nil && err != nil {
		// The parent relay is unavailable which means the session will
		// eventually close.
		s.logger.Warn().Err(err).Msg("parent is closing")

		code := status.Code(xerrors.Unwrap(err))

		fn(xerrors.Errorf("session %v is closing: %v", s.me, code))

		return
	}
	if err != nil {
		s.logger.Warn().Err(err).Msg("relay failed to send")

		// Try to send the packet through a different route.
		s.onFailure(ctx, relay.Distant(), pkt, fn)

		return
	}

	for _, err := range ack.Errors {
		// Note: it would be possible to use this ack feedback to further
		// improve the correction of the routes by retrying here too.
		fn(xerrors.New(err))
	}
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

	hs, err := s.table.Prelude(addr).Serialize(s.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize handshake: %v", err)
	}

	md := s.md.Copy()
	md.Set("handshake", string(hs))

	ctx = metadata.NewOutgoingContext(ctx, md)

	cl := ptypes.NewOverlayClient(conn)

	stream, err := cl.Stream(ctx, grpc.WaitForReady(false))
	if err != nil {
		return nil, xerrors.Errorf("client: %v", err)
	}

	// 2. Wait for the header event to confirm the stream is up and running.
	_, err = stream.Header()
	if err != nil {
		return nil, err
	}

	// 3. Create and run the relay to respond to incoming packets.
	newRelay, err := NewRelay(stream, addr, s.context, s.connMgr, s.md)
	if err != nil {
		return nil, err
	}

	s.relays[addr] = newRelay
	s.Add(1)

	go func() {
		defer func() {
			s.Lock()
			delete(s.relays, addr)
			s.Unlock()

			newRelay.Close()

			// Let the manager know it can close the connection if necessary.
			s.connMgr.Release(addr)

			s.traffic.LogRelayClosed(addr)
			s.logger.Trace().Err(err).Msg("relay closed")
			s.Done()
		}()

		for {
			_, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if status.Code(err) == codes.Canceled {
				return
			}
			if err != nil {
				s.logger.
					Err(err).
					Str("to", addr.String()).
					Msg("relay failed to receive")

				// Relay has lost the connection, therefore we announce the
				// address as unreachable.
				s.table.OnFailure(addr)

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

func (s *session) onFailure(ctx context.Context, gateway mino.Address, p router.Packet, fn func(error)) {
	err := s.table.OnFailure(gateway)
	if err != nil {
		fn(xerrors.Errorf("no route to %v: %v", gateway, err))
		return
	}

	// Retry to send the packet after the announcement of a link failure. This
	// recursive call will eventually end by either a success, or a total
	// failure to send the packet.
	s.sendPacket(ctx, p, fn)
}

// PacketStream is a gRPC stream to send and receive protobuf packets.
type PacketStream interface {
	Context() context.Context
	Send(*ptypes.Packet) error
	Recv() (*ptypes.Packet, error)
}

// UnicastRelay is a relay to a distant peer that is using unicast to send
// packets so that it can learn about failures.
//
// - implements session.Relay
type unicastRelay struct {
	sync.Mutex
	md      metadata.MD
	gw      mino.Address
	stream  PacketStream
	connMgr ConnectionManager
	conn    grpc.ClientConnInterface
	context serde.Context
}

// NewRelay returns a new relay that will send messages to the gateway through
// unicast requests.
func NewRelay(stream PacketStream, gw mino.Address, ctx serde.Context,
	connMgr ConnectionManager, md metadata.MD) (Relay, error) {

	conn, err := connMgr.Acquire(gw)
	if err != nil {
		return nil, err
	}

	r := &unicastRelay{
		md:      md,
		gw:      gw,
		stream:  stream,
		context: ctx,
		connMgr: connMgr,
		conn:    conn,
	}

	return r, nil
}

func (r *unicastRelay) Distant() mino.Address {
	return r.gw
}

func (r *unicastRelay) Stream() PacketStream {
	return r.stream
}

// Send implements session.Relay. It sends the message to the distant peer.
func (r *unicastRelay) Send(ctx context.Context, p router.Packet) (*ptypes.Ack, error) {
	data, err := p.Serialize(r.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize: %v", err)
	}

	client := ptypes.NewOverlayClient(r.conn)

	ctx = metadata.NewOutgoingContext(ctx, r.md)

	ack, err := client.Forward(ctx, &ptypes.Packet{Serialized: data})
	if err != nil {
		return nil, xerrors.Errorf("client: %w", err)
	}

	return ack, nil
}

// Close implements session.Relay. It closes the stream.
func (r *unicastRelay) Close() error {
	r.connMgr.Release(r.gw)

	stream, ok := r.stream.(ptypes.Overlay_StreamClient)
	if ok {
		err := stream.CloseSend()
		if err != nil {
			return xerrors.Errorf("failed to close stream: %v", err)
		}
	}

	return nil
}

// StreamRelay is a relay to a distant peer that will send the packets through a
// stream, which means that it assumes the packet arrived if send is successful.
//
// - implements session.Relay
type streamRelay struct {
	gw      mino.Address
	stream  PacketStream
	context serde.Context
}

// NewStreamRelay creates a new relay that will send the packets through the
// stream.
func NewStreamRelay(gw mino.Address, stream PacketStream, ctx serde.Context) Relay {
	return &streamRelay{
		gw:      gw,
		stream:  stream,
		context: ctx,
	}
}

func (r *streamRelay) Distant() mino.Address {
	return r.gw
}

func (r *streamRelay) Stream() PacketStream {
	return r.stream
}

func (r *streamRelay) Send(ctx context.Context, p router.Packet) (*ptypes.Ack, error) {
	data, err := p.Serialize(r.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize: %v", err)
	}

	err = r.stream.Send(&ptypes.Packet{Serialized: data})
	if err != nil {
		return nil, err
	}

	return &ptypes.Ack{}, nil
}

func (r *streamRelay) Close() error {
	return nil
}
