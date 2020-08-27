package minogrpc

import (
	context "context"
	"sync"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

// OutContext is wrapper for an envelope and the error channel.
type OutContext struct {
	Envelope *Envelope
	Done     chan error
}

type sender struct {
	me       mino.Address
	context  serde.Context
	clients  map[mino.Address]chan OutContext
	receiver receiver
	traffic  *traffic

	router router.Router
	// gateway is the address of the one that contacted us and opened the
	// stream. We need to know it so that when we need to contact this address,
	// we can just reply, and not create a new connection.
	gateway mino.Address

	relays      *sync.Map
	connFactory ConnectionFactory
	uri         string
}

func (s sender) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, 1)
	defer close(errs)

	data, err := msg.Serialize(s.context)
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

	from, err := s.me.MarshalText()
	if err != nil {
		errs <- xerrors.Errorf("couldn't marshal source address: %v", err)
		return errs
	}

	mebuf, err := s.me.MarshalText()
	if err != nil {
		errs <- xerrors.Errorf("failed to marhal my address: %v", err)
		return errs
	}

	relayCtx := metadata.NewOutgoingContext(s.receiver.ctx, metadata.Pairs(
		headerURIKey, s.uri, headerGatewayKey, string(mebuf)))

	envelope := &Envelope{
		To: to,
		Message: &Message{
			From:    from,
			Payload: data,
		},
	}

	dispatched, err := dispatchMessage(*envelope, s)
	if err != nil {
		errs <- xerrors.Errorf("failed to dispatch message: %v", err)
		return errs
	}

	sendToRelays(relayCtx, s, dispatched)

	return errs
}

type receiver struct {
	context        serde.Context
	factory        serde.Factory
	addressFactory mino.AddressFactory
	errs           chan error
	queue          Queue

	ctx context.Context
}

func (r receiver) appendMessage(msg *Message) {
	// This *must* be non-blocking to avoid the relay to stall.
	r.queue.Push(msg)
}

func (r receiver) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
	var msg *Message
	select {
	case msg = <-r.queue.Channel():
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	payload, err := r.factory.Deserialize(r.context, msg.GetPayload())
	if err != nil {
		return nil, nil, err
	}

	from := r.addressFactory.FromText(msg.GetFrom())

	return from, payload, nil
}

// Queue is an interface to queue messages.
type Queue interface {
	Channel() <-chan *Message
	Push(*Message)
}

// NonBlockingQueue is an implementation of a queue that makes sure pushing a
// message will never hang.
type NonBlockingQueue struct {
	sync.Mutex
	working sync.WaitGroup
	buffer  []*Message
	running bool
	ch      chan *Message
}

func newNonBlockingQueue() *NonBlockingQueue {
	return &NonBlockingQueue{
		ch: make(chan *Message, 1),
	}
}

// Channel implements minogrpc.Queue. It returns the message channel.
func (q *NonBlockingQueue) Channel() <-chan *Message {
	return q.ch
}

// Push implements minogrpc.Queue. It appends the message to the queue without
// blocking.
func (q *NonBlockingQueue) Push(msg *Message) {
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
