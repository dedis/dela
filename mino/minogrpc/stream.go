package minogrpc

import (
	context "context"
	"sync"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
)

type sender struct {
	encoder        encoding.ProtoMarshaler
	me             mino.Address
	addressFactory mino.AddressFactory
	rting          routing.Routing
	clients        map[mino.Address]chan *Envelope
	receiver       *receiver
	traffic        *traffic
}

func (s sender) Send(msg proto.Message, addrs ...mino.Address) <-chan error {
	msgAny, err := s.encoder.MarshalAny(msg)
	if err != nil {
		panic(err)
	}

	to := make([][]byte, len(addrs))
	for i, addr := range addrs {
		buffer, err := addr.MarshalText()
		if err != nil {
			panic(err)
		}

		to[i] = buffer
	}

	env := &Envelope{
		To: to,
		Message: &Message{
			Payload: msgAny,
		},
	}

	if s.me != nil {
		buffer, err := s.me.MarshalText()
		if err != nil {
			panic(err)
		}

		env.Message.From = buffer
	}

	s.sendEnvelope(env)
	errs := make(chan error, 1)
	if err != nil {
		errs <- err
	}
	close(errs)

	return errs
}

func (s sender) sendEnvelope(envelope *Envelope) {
	outs := map[mino.Address]*Envelope{}
	for _, to := range envelope.GetTo() {
		addr := s.addressFactory.FromText(to)

		if s.me.Equal(addr) {
			s.traffic.logRcv(nil, s.me, envelope, "")

			s.receiver.appendMessage(envelope.GetMessage())
		} else {
			relay := s.rting.GetRoute(s.me, addr)

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
			panic("that's not possible")
		}

		s.traffic.logSend(s.me, relay, env, "")

		ch <- env
	}
}

// Queue is an interface to queue messages.
type Queue interface {
	Pop() *Message
	Push(*Message)
}

// NonBlockingQueue is an implementation of a queue that makes sure pushing a
// message will never hang.
type NonBlockingQueue struct {
	sync.Mutex
	buffer []*Message
	ch     chan *Message
}

// Pop implements minogrpc.Queue. It returns the oldest message of the queue or
// wait for the next one.
func (q *NonBlockingQueue) Pop() *Message {
	q.Lock()

	if len(q.buffer) > 0 {
		res := q.buffer[0]
		q.buffer = q.buffer[1:]
		q.Unlock()
		return res
	}

	q.Unlock()

	return <-q.ch
}

// Push implements minogrpc.Queue. It appends the message to the queue.
func (q *NonBlockingQueue) Push(msg *Message) {
	select {
	case q.ch <- msg:
		// Message went through !
	default:
		q.Lock()
		q.buffer = append(q.buffer, msg)
		q.Unlock()
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

func (r receiver) Recv(context.Context) (mino.Address, proto.Message, error) {
	msg := r.queue.Pop()

	payload, err := r.encoder.UnmarshalDynamicAny(msg.GetPayload())
	if err != nil {
		return nil, nil, err
	}

	from := r.addressFactory.FromText(msg.GetFrom())

	return from, payload, nil
}
