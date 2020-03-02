package minoch

import (
	"context"
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/mino"
)

// RPC is an implementation of the mino.RPC interface.
type RPC struct {
	manager *Manager
	path    string
	h       mino.Handler
}

// Call sends the message to all participants and gather their reply.
func (c RPC) Call(req proto.Message, addrs ...*mino.Address) (<-chan proto.Message, <-chan error) {
	out := make(chan proto.Message, len(addrs))
	errs := make(chan error, len(addrs))

	go func() {
		for _, addr := range addrs {
			peer := c.manager.get(addr.GetId())
			if peer != nil {
				resp, err := peer.rpcs[c.path].h.Process(req)
				if err != nil {
					errs <- err
				}

				if resp != nil {
					out <- resp
				}
			}
		}

		close(out)
	}()

	return out, errs
}

// Stream opens a stream. The caller is responsible for cancelling the context
// to close the stream.
func (c RPC) Stream(ctx context.Context, addrs ...*mino.Address) (mino.Sender, mino.Receiver) {
	in := make(chan *mino.Envelope)
	out := make(chan *mino.Envelope, 1)
	errs := make(chan error, 1)

	outs := make(map[string]receiver)

	for _, addr := range addrs {
		ch := make(chan *mino.Envelope, 1)
		outs[addr.GetId()] = receiver{out: ch}

		peer := c.manager.instances[addr.GetId()]

		go func(r receiver) {
			s := sender{
				addr: peer.Address(),
				in:   in,
			}

			err := peer.rpcs[c.path].h.Stream(s, r)
			if err != nil {
				errs <- err
			}
		}(outs[addr.GetId()])
	}

	orchSender := sender{addr: &mino.Address{}, in: in}
	orchRecv := receiver{out: out, errs: errs}

	go func() {
		for {
			select {
			case <-ctx.Done():
				// closes the orchestrator..
				close(out)
				// closes the participants..
				for _, r := range outs {
					close(r.out)
				}
				return
			case env := <-in:
				for _, to := range env.GetTo() {
					if to.GetId() == "" {
						orchRecv.out <- env
					} else {
						outs[to.GetId()].out <- env
					}
				}
			}
		}
	}()

	return orchSender, orchRecv
}

type sender struct {
	addr *mino.Address
	in   chan *mino.Envelope
}

func (s sender) Send(msg proto.Message, addrs ...*mino.Address) error {
	a, err := ptypes.MarshalAny(msg)
	if err != nil {
		return err
	}

	go func() {
		s.in <- &mino.Envelope{
			From:    s.addr,
			To:      addrs,
			Message: a,
		}
	}()

	return nil
}

type receiver struct {
	out  chan *mino.Envelope
	errs chan error
}

func (r receiver) Recv(ctx context.Context) (*mino.Address, proto.Message, error) {
	select {
	case env := <-r.out:
		var da ptypes.DynamicAny
		err := ptypes.UnmarshalAny(env.GetMessage(), &da)
		if err != nil {
			return nil, nil, err
		}

		return env.From, da.Message, nil
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, errors.New("timeout")
	}
}
