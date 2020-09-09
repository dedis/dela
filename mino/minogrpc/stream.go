package minogrpc

import (
	context "context"
	"io"
	"sync"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
)

type sender struct {
	*overlay

	sme         mino.Address // 'me' refers to overlay.me
	traffic     *traffic
	connFactory ConnectionFactory
	table       router.RoutingTable

	// the uri is a uniq identifier for the rpc
	uri string

	// the receiver is used when one of our connections receive a message for us
	receiver receiver

	// a uniq stream id should be generated for each stream. This streamID is
	// used by the overlay to store and get the stream sessions.
	streamID string

	// gateway is the address of the one that contacted us and opened the
	// stream. We need to know it so that when we need to contact this address,
	// we can just reply, and not create a new connection.
	gateway mino.Address

	// used to notify when the context is done
	done chan struct{}

	// used to create and close relays
	lock *sync.Mutex

	// holds the relays, should be carefully handled with the lock Mutex
	connections map[mino.Address]safeRelay

	relaysWait *sync.WaitGroup
}

func (s sender) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, 1)
	defer close(errs)

	data, err := msg.Serialize(s.receiver.context)
	if err != nil {
		errs <- xerrors.Errorf("couldn't marshal message: %v", err)
		return errs
	}

	to := make([][]byte, len(addrs))
	for i, addr := range addrs {
		buffer, err := addr.MarshalText()
		if err != nil {
			errs <- xerrors.Errorf("couldn't marshal address: %v", err)
			return errs
		}

		to[i] = buffer
	}

	mebuf, err := s.sme.MarshalText()
	if err != nil {
		errs <- xerrors.Errorf("failed to marhal my address: %v", err)
		return errs
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.done
		cancel()
	}()

	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		headerURIKey, s.uri,
		headerGatewayKey, string(mebuf),
		headerStreamIDKey, s.streamID))

	packet := s.table.Make(s.sme, addrs, data)

	err = s.sendPacket(ctx, packet)
	if err != nil {
		errs <- xerrors.Errorf("failed to send to relays: %v", err)
		return errs
	}

	return errs
}

func (s sender) send(to mino.Address, proto *Packet) error {
	conn, found := s.connections[to]
	if !found {
		return xerrors.Errorf("expected to find out for client '%s'", to)
	}

	err := conn.Send(proto)
	if err != nil {
		return xerrors.Errorf("couldn't send to address: %v", err)
	}

	return nil
}

func (s sender) closeRelays() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for addr, streamConn := range s.connections {
		err := streamConn.close()
		if err != nil {
			s.receiver.logger.Warn().Msgf("failed to close relay '%s': %v", addr, err)
		} else {
			s.traffic.addEvent("close", s.sme, addr)
		}
	}
}

// sendPacket creates the relays if needed and sends the packets accordingly.
func (s sender) sendPacket(ctx context.Context, packet router.Packet) error {
	me := packet.Slice(s.sme)
	if me != nil {
		s.receiver.appendMessage(me)
	}

	p := packet.Slice(newRootAddress())

	packets, err := s.table.Forward(packet)
	if err != nil {
		return xerrors.Errorf("failed to route packet: %v", err)
	}

	if p != nil {
		// TODO: Am I replacing something there?
		packets[newRootAddress()] = p
	}

	for to, packet := range packets {

		// s.receiver.logger.Info().Msgf("sending to %s, dest: %v",
		// to, packet.GetDestination())

		// The router should send nil when it doesn't know the 'from' or the
		// 'to'.
		if to == nil {
			to = newRootAddress()
		}

		if to.Equal(s.sme) {
			s.receiver.appendMessage(packet)
			continue
		}

		// If we are not the orchestrator and we must send a message to it, our
		// only option is to send the message back to our gateway.
		if to.Equal(newRootAddress()) {
			to = s.gateway
		}

		// If not already done, setup the connection to this address
		err := s.SetupRelay(ctx, to)
		if err != nil {
			return xerrors.Errorf("failed to setup relay to '%s': %v",
				to, err)
		}

		buf, err := packet.Serialize(s.receiver.context)
		if err != nil {
			return xerrors.Errorf("failed to serialize packet: %v", err)
		}

		proto := &Packet{
			Serialized: buf,
		}

		s.traffic.logSend(ctx, s.sme, to, packet)

		err = s.send(to, proto)
		if err != nil {
			s.receiver.logger.Warn().Msgf("failed to send to relay '%s': %v",
				to, err)
		}
	}

	return nil
}

func (s sender) SetupRelay(ctx context.Context, addr mino.Address) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, found := s.connections[addr]
	if !found {
		s.receiver.logger.Trace().Msgf("opening a relay to %s", addr)

		conn, err := s.connFactory.FromAddress(addr)
		if err != nil {
			return xerrors.Errorf("failed to create addr: %v", err)
		}

		cl := NewOverlayClient(conn)

		r, err := cl.Stream(ctx, grpc.WaitForReady(false))
		if err != nil {
			// TODO: this error is actually never seen by the user
			s.receiver.logger.Fatal().Msgf(
				"failed to call stream for relay '%s': %v", addr, err)
			return xerrors.Errorf("'%s' failed to call stream for "+
				"relay '%s': %v", s.sme, addr, err)
		}

		hs, err := s.table.Prelude(addr).Serialize(s.context)
		if err != nil {
			return err
		}

		err = r.Send(&Packet{Serialized: hs})
		if err != nil {
			return err
		}

		relay := &streamConn{client: r, conn: conn}

		s.traffic.addEvent("open", s.sme, addr)

		s.connections[addr] = relay
		go s.listenStream(ctx, relay, addr)
	}

	return nil
}

// listenStream listens for new messages that would come from a stream that we
// set up and either notify us, or relay the message. This function is blocking.
func (s sender) listenStream(streamCtx context.Context, stream relayable,
	distantAddr mino.Address) {

	s.relaysWait.Add(1)
	defer s.relaysWait.Done()

	for {
		select {
		// Allows us to perform an early stopping in case the contex is already
		// done.
		case <-streamCtx.Done():
			return
		default:
			proto, err := stream.Recv()
			if err == io.EOF {
				return
			}
			status, ok := status.FromError(err)
			if ok && status.Code() == codes.Canceled {
				return
			}
			if err != nil {
				// TODO: this error is actually never seen by the user
				s.receiver.logger.Warn().Msgf("stream failed to receive: %v", err)
				s.receiver.errs <- xerrors.Errorf("failed to receive: %v", err)
				return
			}

			packet, err := s.router.GetPacketFactory().PacketOf(s.receiver.context, proto.Serialized)
			if err != nil {
				s.receiver.errs <- xerrors.Errorf("failed to deserialize packet: %v", err)
				return
			}

			s.traffic.logRcv(stream.Context(), distantAddr, s.sme)

			err = s.sendPacket(streamCtx, packet)
			if err != nil {
				s.receiver.errs <- xerrors.Errorf("failed to send to "+
					"dispatched relays: %v", err)
				return
			}
		}
	}
}

type receiver struct {
	context        serde.Context
	factory        serde.Factory
	addressFactory mino.AddressFactory
	errs           chan error
	queue          Queue
	logger         zerolog.Logger
}

func (r receiver) appendMessage(msg router.Packet) {
	// This *must* be non-blocking to avoid the relay to stall.
	r.queue.Push(msg)
}

func (r receiver) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
	var packet router.Packet
	select {
	case packet = <-r.queue.Channel():
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	payload := packet.GetMessage()

	msg, err := r.factory.Deserialize(r.context, payload)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to deserialize message: %v", err)
	}

	return packet.GetSource(), msg, nil
}

// Queue is an interface to queue messages.
type Queue interface {
	Channel() <-chan router.Packet
	Push(router.Packet)
}

// NonBlockingQueue is an implementation of a queue that makes sure pushing a
// message will never hang.
type NonBlockingQueue struct {
	sync.Mutex
	working sync.WaitGroup
	buffer  []router.Packet
	running bool
	ch      chan router.Packet
}

func newNonBlockingQueue() *NonBlockingQueue {
	return &NonBlockingQueue{
		ch: make(chan router.Packet, 1),
	}
}

// Channel implements minogrpc.Queue. It returns the message channel.
func (q *NonBlockingQueue) Channel() <-chan router.Packet {
	return q.ch
}

// Push implements minogrpc.Queue. It appends the message to the queue without
// blocking.
func (q *NonBlockingQueue) Push(msg router.Packet) {
	select {
	case q.ch <- msg:
		// Message went through !
	default:
		q.Lock()
		// TODO: memory control
		q.buffer = append(q.buffer, msg)

		if !q.running {
			q.running = true
			go q.pushAndWait()
		}
		q.Unlock()
	}
}

func (q *NonBlockingQueue) pushAndWait() {
	q.working.Add(1)
	defer q.working.Done()

	for {
		q.Lock()
		if len(q.buffer) == 0 {
			q.running = false
			q.Unlock()
			return
		}

		msg := q.buffer[0]
		q.buffer = q.buffer[1:]
		q.Unlock()

		// Wait for the channel to be available to writings.
		q.ch <- msg
	}
}
