package minogrpc

import (
	context "context"
	"sync"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
)

// OutContext is wrapper for an envelope and the error channel.
type OutContext struct {
	Envelope *Envelope
	Done     chan error
}

type sender struct {
	encoder        encoding.ProtoMarshaler
	me             mino.Address
	addressFactory mino.AddressFactory
	clients        map[mino.Address]chan OutContext
	receiver       *receiver
	traffic        *traffic

	// Routing parameters to differentiate an orchestrator from a relay. The
	// gateway will be used to route any messages and the routing will define
	// proper routes.
	rting   routing.Routing
	gateway mino.Address
}

func (s sender) Send(msg proto.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, 1)
	defer close(errs)

	msgAny, err := s.encoder.MarshalAny(msg)
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

	env := &Envelope{
		To: to,
		Message: &Message{
			From:    from,
			Payload: msgAny,
		},
	}

	if s.me != nil {
		buffer, err := s.me.MarshalText()
		if err != nil {
			errs <- xerrors.Errorf("couldn't marshal source address: %v", err)
			return errs
		}

		env.Message.From = buffer
	}

	s.sendEnvelope(env, errs)

	return errs
}

func (s sender) sendEnvelope(envelope *Envelope, errs chan error) {
	outs := map[mino.Address]*Envelope{}
	for _, to := range envelope.GetTo() {
		addr := s.addressFactory.FromText(to)

		if s.me.Equal(addr) {
			s.receiver.appendMessage(envelope.GetMessage())
		} else {
			var relay mino.Address
			if s.rting != nil {
				relay = s.rting.GetRoute(s.me, addr)
				if relay == nil {
					relay = s.rting.GetParent(s.me)
				}
			} else if s.gateway != nil {
				relay = s.gateway
			}

			env, ok := outs[relay]
			if !ok {
				env = &Envelope{Message: envelope.GetMessage()}
				outs[relay] = env
			}

			env.To = append(env.To, to)
		}
	}

	for relay, env := range outs {
		ch := s.clients[relay]
		if ch == nil {
			errs <- xerrors.Errorf("inconsistent routing to <%v>", relay)
			continue
		}

		done := make(chan error, 1)
		ch <- OutContext{Envelope: env, Done: done}

		// Wait for an potential error from grpc when sending the envelope.
		err := <-done
		if err != nil {
			errs <- xerrors.Errorf("couldn't send to relay: %v", err)
		}
	}
}

type receiver struct {
	encoder        encoding.ProtoMarshaler
	addressFactory mino.AddressFactory
	errs           chan error
	queue          Queue
}

func (r receiver) appendMessage(msg *Message) {
	// This *must* be non-blocking to avoid the relay to stall.
	r.queue.Push(msg)
}

func (r receiver) Recv(ctx context.Context) (mino.Address, proto.Message, error) {
	var msg *Message
	select {
	case msg = <-r.queue.Channel():
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	payload, err := r.encoder.UnmarshalDynamicAny(msg.GetPayload())
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
