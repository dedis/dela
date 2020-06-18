package minoch

import (
	"context"
	"io"
	"math"
	"sync"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/json"
	"golang.org/x/xerrors"
)

// Envelope is the wrapper to send messages through streams.
type Envelope struct {
	to      []mino.Address
	from    address
	message []byte
}

// RPC is an implementation of the mino.RPC interface.
type RPC struct {
	manager *Manager
	addr    mino.Address
	path    string
	h       mino.Handler
	context serdeng.Context
	factory serdeng.Factory
}

// Call sends the message to all participants and gather their reply. The
// context in the scope of channel communication as there is no blocking I/O.
func (c RPC) Call(ctx context.Context, req serdeng.Message,
	players mino.Players) (<-chan serdeng.Message, <-chan error) {

	out := make(chan serdeng.Message, players.Len())
	errs := make(chan error, players.Len())

	data, err := req.Serialize(json.NewContext())

	if err != nil {
		errs <- xerrors.Errorf("couldn't serialize: %v", err)
		close(out)
		return out, errs
	}

	wg := sync.WaitGroup{}
	wg.Add(players.Len())
	iter := players.AddressIterator()
	for iter.HasNext() {
		peer, err := c.manager.get(iter.GetNext())
		if err != nil {
			errs <- xerrors.Errorf("couldn't find peer: %v", err)
			continue
		}

		go func(m *Minoch) {
			defer wg.Done()

			msg, err := c.factory.Deserialize(json.NewContext(), data)
			if err != nil {
				errs <- xerrors.Errorf("couldn't deserialize: %v", err)
				return
			}

			req := mino.Request{
				Address: c.addr,
				Message: msg,
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
				errs <- xerrors.Errorf("couldn't process request: %v", err)
			}

			if resp != nil {
				out <- resp
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
func (c RPC) Stream(ctx context.Context, memship mino.Players) (mino.Sender, mino.Receiver, error) {
	in := make(chan Envelope, 100)
	out := make(chan Envelope, 100)
	errs := make(chan error, 1)

	outs := make(map[string]receiver)

	iter := memship.AddressIterator()
	for iter.HasNext() {
		addr := iter.GetNext()

		peer, err := c.manager.get(addr)
		if err != nil {
			return nil, nil, xerrors.Errorf("couldn't find peer: %v", err)
		}

		ch := make(chan Envelope, 1)
		outs[addr.String()] = receiver{
			out:     ch,
			context: c.context,
			factory: c.factory,
		}

		go func(r receiver) {
			s := sender{
				addr:    peer.GetAddress(),
				in:      in,
				context: c.context,
			}

			err := peer.rpcs[c.path].h.Stream(s, r)
			if err != nil {
				// TODO:
				errs <- xerrors.Errorf("couldn't process: %v", err)
			}
		}(outs[addr.String()])
	}

	orchAddr := c.addr.(address)
	orchAddr.orchestrator = true

	orchSender := sender{
		addr:    orchAddr,
		in:      in,
		context: c.context,
	}

	orchRecv := receiver{
		out:     out,
		errs:    errs,
		context: c.context,
		factory: c.factory,
	}

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
					if to.(address).orchestrator {
						orchRecv.out <- env
					} else {
						outs[to.String()].out <- env
					}
				}
			}
		}
	}()

	return orchSender, orchRecv, nil
}

type sender struct {
	addr    mino.Address
	in      chan Envelope
	context serdeng.Context
}

func (s sender) Send(msg serdeng.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, int(math.Max(1, float64(len(addrs)))))

	data, err := msg.Serialize(s.context)
	if err != nil {
		errs <- xerrors.Errorf("couldn't marshal message: %v", err)
		close(errs)
		return errs
	}

	go func() {
		s.in <- Envelope{
			from:    s.addr.(address),
			to:      addrs,
			message: data,
		}
		close(errs)
	}()

	return errs
}

type receiver struct {
	out     chan Envelope
	errs    chan error
	context serdeng.Context
	factory serdeng.Factory
}

func (r receiver) Recv(ctx context.Context) (mino.Address, serdeng.Message, error) {
	select {
	case env, ok := <-r.out:
		if !ok {
			return nil, nil, io.EOF
		}

		msg, err := r.factory.Deserialize(r.context, env.message)
		if err != nil {
			return nil, nil, xerrors.Errorf("couldn't deserialize: %v", err)
		}

		return env.from, msg, nil
	case err := <-r.errs:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, xerrors.New("timeout")
	}
}
