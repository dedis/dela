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

// ConnectionFactory is a factory to open connection to distant addresses.
type ConnectionFactory interface {
	FromAddress(mino.Address) (grpc.ClientConnInterface, error)
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
	connFac ConnectionFactory
}

func NewSession(
	md metadata.MD,
	stream PacketStream,
	me mino.Address,
	table router.RoutingTable,
	msgFac serde.Factory,
	pktFac router.PacketFactory,
	ctx serde.Context,
	connFac ConnectionFactory,
) Session {
	return &session{
		md:      md,
		me:      me,
		gateway: NewRelay(stream, pktFac, ctx),
		errs:    make(chan error, 1),
		table:   table,
		msgFac:  msgFac,
		pktFac:  pktFac,
		context: ctx,
		queue:   newNonBlockingQueue(),
		relays:  make(map[mino.Address]Relay),
		connFac: connFac,
	}
}

func (s *session) Listen() {
	defer func() {
		err := s.close()
		if err != nil {
			s.errs <- err
		}

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
			s.errs <- xerrors.Errorf("failed to send to dispatched relays: %v", err)
			return
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

func (s *session) close() error {
	s.Lock()
	defer s.Unlock()

	for _, relay := range s.relays {
		err := relay.Close()
		if err != nil {
			return err
		}
	}

	return nil
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

	// TODO: connection manager
	conn, err := s.connFac.FromAddress(addr)
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

	relay = NewRelay(r, s.pktFac, s.context)

	s.relays[addr] = relay

	go func() {
		for {
			pkt, err := relay.Recv()
			if err != nil {
				s.errs <- err
				return
			}

			err = s.sendPacket(ctx, pkt)
			if err != nil {
				s.errs <- err
				return
			}
		}
	}()

	dela.Logger.Info().
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

func NewRelay(stream PacketStream, fac router.PacketFactory, ctx serde.Context) Relay {
	return &relay{
		stream:    stream,
		packetFac: fac,
		context:   ctx,
	}
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

		dela.Logger.Info().Err(err).Msg("relay has closed")
	}

	return nil
}
