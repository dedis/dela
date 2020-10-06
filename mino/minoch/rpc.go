package minoch

import (
	"context"
	"io"
	"math"
	"sync"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
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
	context serde.Context
	factory serde.Factory
	filters []Filter
}

// Call sends the message to all participants and gather their reply. The
// context is ignored in the scope of channel communication as there is no
// blocking I/O. The response channel will receive n responses for n players and
// be closed eventually.
func (c RPC) Call(ctx context.Context,
	req serde.Message, players mino.Players) (<-chan mino.Response, error) {

	data, err := req.Serialize(json.NewContext())
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize: %v", err)
	}

	out := make(chan mino.Response, players.Len())

	wg := sync.WaitGroup{}
	wg.Add(players.Len())

	iter := players.AddressIterator()

	for iter.HasNext() {
		peer, err := c.manager.get(iter.GetNext())
		if err != nil {
			// Abort everything if a peer is missing.
			return nil, xerrors.Errorf("couldn't find peer: %v", err)
		}

		go func(m *Minoch) {
			defer wg.Done()

			from := peer.GetAddress()

			msg, err := c.factory.Deserialize(json.NewContext(), data)
			if err != nil {
				resp := mino.NewResponseWithError(
					from,
					xerrors.Errorf("couldn't deserialize: %v", err),
				)

				out <- resp
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
				resp := mino.NewResponseWithError(
					from,
					xerrors.Errorf("unknown rpc %s", c.path),
				)

				out <- resp
				return
			}

			if !rpc.runFilters(req) {
				// Message is dropped by one of the filter.
				return
			}

			resp, err := rpc.h.Process(req)
			if err != nil {
				resp := mino.NewResponseWithError(
					from,
					xerrors.Errorf("couldn't process request: %v", err),
				)

				out <- resp
				return
			}

			out <- mino.NewResponse(from, resp)
		}(peer)
	}

	// Only wait if all the requests have been correctly started.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

func (c RPC) runFilters(req mino.Request) bool {
	for _, filter := range c.filters {
		if !filter(req) {
			return false
		}
	}

	return true
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
	context serde.Context
}

func (s sender) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
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
	context serde.Context
	factory serde.Factory
}

func (r receiver) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
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
		return nil, nil, ctx.Err()
	}
}
