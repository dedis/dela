// This file contains the orchestrator side of the RPC implementation.
//
// Documentation Last Review: 06.10.2020
//

package minoch

import (
	"context"
	"io"
	"math"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// bufSize defines the buffer size of channels used to store messages
var bufSize = 10000

// Envelope is the wrapper to send messages through streams.
type Envelope struct {
	to      []mino.Address
	from    address
	message []byte
}

// RPC implements a remote procedure call that is calling its peers using the
// channels registered by the manager.
//
// - implements mino.RPC
type RPC struct {
	manager *Manager
	addr    mino.Address
	path    string
	h       mino.Handler
	context serde.Context
	factory serde.Factory
	filters []Filter
}

// Call implements mino.RPC. It sends the message to all participants and
// gathers their replies. The context is ignored in the scope of channel
// communication as there is no blocking I/O. The response channel will receive
// n responses for n players and be closed eventually.
func (c RPC) Call(ctx context.Context,
	req serde.Message, players mino.Players) (<-chan mino.Response, error) {

	data, err := req.Serialize(c.context)
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

			msg, err := c.factory.Deserialize(c.context, data)
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

// Stream implements mino.RPC. It simulates the stream by using the orchestrator
// as the router for all the messages. They are redirected to the channel
// associated with the address.
func (c RPC) Stream(ctx context.Context, memship mino.Players) (mino.Sender, mino.Receiver, error) {
	in := make(chan Envelope, bufSize)
	out := make(chan Envelope, bufSize)
	errs := make(chan error, 1000)

	outs := make(map[string]receiver)

	iter := memship.AddressIterator()
	for iter.HasNext() {
		addr := iter.GetNext()

		peer, err := c.manager.get(addr)
		if err != nil {
			return nil, nil, xerrors.Errorf("couldn't find peer: %v", err)
		}

		ch := make(chan Envelope, bufSize*100)
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
					output := orchRecv.out
					if !to.(address).orchestrator {
						output = outs[to.String()].out
					}

					// FIXME: use a crychan
					select {
					case output <- env:
					default:
						dela.Logger.Warn().Str("to", to.String()).
							Str("from", env.from.String()).Msg("full")
						output <- env
					}
				}
			}
		}
	}()

	return orchSender, orchRecv, nil
}

// Sender implements the sender associated with a stream.
//
// - implements mino.Sender
type sender struct {
	addr    mino.Address
	in      chan Envelope
	context serde.Context
}

// Send implements mino.Sender. It sends the message to all the addresses and
// returns a channel that will be populated with any error happening. The
// channel is eventually closed after all participants have gotten their
// message.
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

// Receiver implements the receiver side of a stream.
//
// - implements mino.Receiver
type receiver struct {
	out     chan Envelope
	errs    chan error
	context serde.Context
	factory serde.Factory
}

// Recv implements mino.Receiver. It listens for messages until the context is
// done, or a message is received. On a graceful close, the receiver will return
// an EOF error, or the error from the context if it finishes before.
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
