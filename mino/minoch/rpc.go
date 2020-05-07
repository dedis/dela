package minoch

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Envelope is the wrapper to send messages through streams.
type Envelope struct {
	to      []mino.Address
	from    address
	message *any.Any
}

// RPC is an implementation of the mino.RPC interface.
type RPC struct {
	manager *Manager
	addr    mino.Address
	path    string
	h       mino.Handler
}

// Call sends the message to all participants and gather their reply. The
// context in the scope of channel communication as there is no blocking I/O.
func (c RPC) Call(ctx context.Context, req proto.Message,
	players mino.Players) (<-chan proto.Message, <-chan error) {

	out := make(chan proto.Message, players.Len())
	errs := make(chan error, players.Len())

	wg := sync.WaitGroup{}
	wg.Add(players.Len())
	iter := players.AddressIterator()
	for iter.HasNext() {
		peer := c.manager.get(iter.GetNext())
		cloneReq := proto.Clone(req)
		go func(m *Minoch) {
			defer wg.Done()

			if m != nil {
				req := mino.Request{
					Address: c.addr,
					Message: cloneReq,
				}

				m.Lock()
				rpc, ok := m.rpcs[c.path]
				m.Unlock()

				if !ok {
					errs <- xerrors.Errorf("unknown rpc %s", c.path)
					return
				}

				resp, err := rpc.h.Process(req)
				if err != nil {
					errs <- err
				}

				if resp != nil {
					out <- proto.Clone(resp)
				}
			}
		}(peer)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, errs
}

// Stream opens a stream. The caller is responsible for cancelling the context
// to close the stream.
func (c RPC) Stream(ctx context.Context, memship mino.Players) (mino.Sender, mino.Receiver) {
	in := make(chan Envelope)
	out := make(chan Envelope, 1)
	errs := make(chan error, 1)

	outs := make(map[string]receiver)

	iter := memship.AddressIterator()
	for iter.HasNext() {
		addr := iter.GetNext()
		ch := make(chan Envelope, 1)
		outs[addr.String()] = receiver{out: ch}

		peer := c.manager.get(addr)

		go func(r receiver) {
			s := sender{
				addr: peer.GetAddress(),
				in:   in,
			}

			err := peer.rpcs[c.path].h.Stream(s, r)
			if err != nil {
				errs <- err
			}
		}(outs[addr.String()])
	}

	orchSender := sender{addr: address{}, in: in}
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
				for _, to := range env.to {
					if to.String() == "" {
						orchRecv.out <- env
					} else {
						outs[to.String()].out <- env
					}
				}
			}
		}
	}()

	return orchSender, orchRecv
}

type sender struct {
	addr mino.Address
	in   chan Envelope
}

func (s sender) Send(msg proto.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, int(math.Max(1, float64(len(addrs)))))

	a, err := ptypes.MarshalAny(msg)
	if err != nil {
		errs <- err
		close(errs)
		return errs
	}

	go func() {
		s.in <- Envelope{
			from:    s.addr.(address),
			to:      addrs,
			message: a,
		}
	}()

	close(errs)
	return errs
}

type receiver struct {
	out  chan Envelope
	errs chan error
}

func (r receiver) Recv(ctx context.Context) (mino.Address, proto.Message, error) {
	select {
	case env, ok := <-r.out:
		if !ok {
			return nil, nil, io.EOF
		}

		var da ptypes.DynamicAny
		err := ptypes.UnmarshalAny(env.message, &da)
		if err != nil {
			return nil, nil, err
		}

		return env.from, da.Message, nil
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, xerrors.New("timeout")
	}
}
