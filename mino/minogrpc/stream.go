package minogrpc

import (
	context "context"
	"sync"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

type sender struct {
	me          mino.Address
	traffic     *traffic
	router      router.Router
	connFactory ConnectionFactory

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

	mebuf, err := s.me.MarshalText()
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
		headerURIKey, s.uri, headerGatewayKey, string(mebuf),
		headerStreamIDKey, s.streamID))

	packet := s.router.MakePacket(s.me, addrs, data)
	ser, err := packet.Serialize(s.receiver.context)
	if err != nil {
		errs <- xerrors.Errorf("failed to serialize packet: %v", err)
		return errs
	}

	proto := &Packet{
		Serialized: ser,
	}

	err = s.sendPacket(ctx, proto)
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
	var msg router.Packet
	select {
	case msg = <-r.queue.Channel():
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	payload, err := msg.GetMessage(r.context, r.factory)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to get message: %v", err)
	}

	return msg.GetSource(), payload, nil
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
