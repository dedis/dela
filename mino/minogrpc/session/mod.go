package session

import (
	"context"
	"io"
	"sync"

	"go.dedis.ch/dela"
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

type ConnectionManager interface {
	Acquire(mino.Address) (grpc.ClientConnInterface, error)
	Release(mino.Address)
}

type Session interface {
	mino.Sender
	mino.Receiver

	Listen()
}

type Relay interface {
	Context() context.Context
	Recv() (router.Packet, error)
	Send(router.Packet) error
	Close() error
}

type session struct {
	sync.Mutex
	sync.WaitGroup
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
}

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
	return &session{
		md:      md,
		me:      me,
		gateway: newRelay(stream, pktFac, ctx),
		errs:    make(chan error, 1),
		table:   table,
		msgFac:  msgFac,
		pktFac:  pktFac,
		context: ctx,
		queue:   newNonBlockingQueue(),
		relays:  make(map[mino.Address]Relay),
		connMgr: connMgr,
	}
}

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

		// TODO: s.traffic.logRcv(stream.Context(), distantAddr, s.sme)

		err = s.sendPacket(s.gateway.Context(), packet)
		if err != nil {
			dela.Logger.Err(err).Msg("failed to send to dispatched relays")
		}
	}
}

func (s *session) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, 1)
	close(errs)

	data, err := msg.Serialize(s.context)
	if err != nil {
		errs <- err
		return errs
	}

	packet := s.table.Make(s.me, addrs, data)

	err = s.sendPacket(s.gateway.Context(), packet)
	if err != nil {
		errs <- err
		return errs
	}

	return errs
}

func (s *session) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case err := <-s.errs:
		return nil, nil, err
	case packet := <-s.queue.Channel():
		msg, err := s.msgFac.Deserialize(s.context, packet.GetMessage())
		if err != nil {
			return nil, nil, err
		}

		return packet.GetSource(), msg, nil
	}
}

func (s *session) close() {
	s.Lock()
	defer s.Unlock()

	for to, relay := range s.relays {
		err := relay.Close()
		dela.Logger.Trace().Err(err).Msg("relay closed")

		// Let the manager know it can close the connection if necessary.
		s.connMgr.Release(to)
	}

	s.Wait()
}

func (s *session) sendPacket(ctx context.Context, p router.Packet) error {
	me := p.Slice(s.me)
	if me != nil {
		s.queue.Push(me)
	}

	if len(p.GetDestination()) == 0 {
		return nil
	}

	routes, err := s.table.Forward(p)
	if err != nil {
		return err
	}

	for addr, packet := range routes {
		var relay Relay
		if addr == nil {
			relay = s.gateway
		} else {
			relay, err = s.setupRelay(ctx, addr)
			if err != nil {
				return err
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

	conn, err := s.connMgr.Acquire(addr)
	if err != nil {
		return nil, xerrors.Errorf("failed to create addr: %v", err)
	}

	cl := ptypes.NewOverlayClient(conn)
	ctx = metadata.NewOutgoingContext(ctx, s.md)

	r, err := cl.Stream(ctx, grpc.WaitForReady(false))
	if err != nil {
		return nil, xerrors.Errorf("'%s' failed to call stream for "+
			"relay '%s': %v", s.me, addr, err)
	}

	hs, err := s.table.Prelude(addr).Serialize(s.context)
	if err != nil {
		return nil, err
	}

	err = r.Send(&ptypes.Packet{Serialized: hs})
	if err != nil {
		return nil, err
	}

	relay = newRelay(r, s.pktFac, s.context)

	s.relays[addr] = relay
	s.Add(1)

	go func() {
		defer s.Done()

		for {
			pkt, err := relay.Recv()
			if err == io.EOF {
				return
			}
			if status.Code(err) == codes.Canceled {
				return
			}
			if err != nil {
				dela.Logger.Err(err).Msg("relay failed")
				return
			}

			err = s.sendPacket(ctx, pkt)
			if err != nil {
				dela.Logger.Err(err).Msg("relay failed to send a packet")
				return
			}
		}
	}()

	dela.Logger.Trace().
		Str("from", s.me.String()).
		Str("to", addr.String()).
		Msg("relay opened")

	return relay, nil
}

type PacketStream interface {
	Context() context.Context
	Send(*ptypes.Packet) error
	Recv() (*ptypes.Packet, error)
}

type relay struct {
	sync.Mutex
	stream    PacketStream
	packetFac router.PacketFactory
	context   serde.Context
}

func newRelay(stream PacketStream, fac router.PacketFactory, ctx serde.Context) *relay {
	r := &relay{
		stream:    stream,
		packetFac: fac,
		context:   ctx,
	}

	return r
}

func (r *relay) Context() context.Context {
	return r.stream.Context()
}

func (r *relay) Recv() (router.Packet, error) {
	proto, err := r.stream.Recv()
	if err != nil {
		return nil, err
	}

	packet, err := r.packetFac.PacketOf(r.context, proto.Serialized)
	if err != nil {
		return nil, err
	}

	return packet, nil
}

func (r *relay) Send(p router.Packet) error {
	data, err := p.Serialize(r.context)
	if err != nil {
		return err
	}

	r.Lock()
	defer r.Unlock()

	err = r.stream.Send(&ptypes.Packet{Serialized: data})
	if err != nil {
		return err
	}

	return nil
}

func (r *relay) Close() error {
	stream, ok := r.stream.(ptypes.Overlay_StreamClient)
	if ok {
		err := stream.CloseSend()
		if err != nil {
			return err
		}
	}

	return nil
}
