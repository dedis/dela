// Package qsc implements the Que Sera Consensus algorithm. At the time this is
// written, the algorithm does *not* support Byzantine behavior. This is
// currently work in progress.
// TODO: link to the paper when published
// TODO: Byzantine behaviors
// TODO: chain integrity
package qsc

import (
	"math/rand"
	"time"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	// EpochTimeout is the maximum amount of time given to an epoch to end
	// before the request is aborted.
	EpochTimeout = 20 * time.Second
)

var protoenc encoding.ProtoMarshaler = encoding.NewProtoEncoder()

// Consensus is an abstraction to send proposals to a network of nodes that will
// decide to include them in the common state.
type Consensus struct {
	ch               chan consensus.Proposal
	closing          chan struct{}
	stopped          chan struct{}
	history          history
	broadcast        broadcast
	historiesFactory historiesFactory
}

// NewQSC returns a new instance of QSC.
func NewQSC(node int64, mino mino.Mino, players mino.Players) (*Consensus, error) {
	bc, err := newBroadcast(node, mino, players)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create broadcast: %v", err)
	}

	return &Consensus{
		ch:               make(chan consensus.Proposal),
		closing:          make(chan struct{}),
		stopped:          make(chan struct{}),
		history:          make(history, 0),
		broadcast:        bc,
		historiesFactory: defaultHistoriesFactory{},
	}, nil
}

// GetChainFactory implements consensus.Consensus. It returns the chain factory.
func (c *Consensus) GetChainFactory() consensus.ChainFactory {
	return nil
}

// GetChain implements consensus.Consensus. It returns the chain that can prove
// the integrity of the proposal with the given identifier.
func (c *Consensus) GetChain(id []byte) (consensus.Chain, error) {
	return nil, nil
}

// Listen implements consensus.Consensus. It returns the actor that provides the
// primitives to send proposals to a network of nodes.
func (c *Consensus) Listen(val consensus.Validator) (consensus.Actor, error) {
	go func() {
		for {
			var proposal consensus.Proposal
			select {
			case <-c.closing:
				fabric.Logger.Trace().Msg("closing")
				close(c.stopped)
				return
			case proposal = <-c.ch:
			default:
				// If the current node does not have anything to propose, it
				// still has to participate so it sends an empty proposal.
				proposal = nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), EpochTimeout)

			go func() {
				// This Go routine is responsible for listening a close event
				// from the actor.
				select {
				case <-ctx.Done():
				case <-c.closing:
					// Cancel the execution of the next time step.
					cancel()
				}
			}()

			err := c.executeRound(ctx, proposal, val)
			if err != nil {
				select {
				case <-c.closing:
				default:
					// Only log if the consensus has not been closed properly.
					fabric.Logger.Err(err).Msg("failed to execute a time step")
				}
			}

			cancel()
		}
	}()

	return actor{ch: c.ch, closing: c.closing}, nil
}

func (c *Consensus) executeRound(
	ctx context.Context,
	prop consensus.Proposal,
	val consensus.Validator,
) error {
	// 1. Choose the message and the random value. The new epoch will be
	// appended to the current history.
	e := epoch{
		// TODO: ask about randomness
		random: rand.Int63(),
	}

	if prop != nil {
		e.hash = prop.GetHash()
	}

	newHistory := make(history, len(c.history), len(c.history)+1)
	copy(newHistory, c.history)
	newHistory = append(newHistory, e)

	// 2. Broadcast our history to the network and get back messages
	// from this time step.
	prepareSet, err := c.broadcast.send(ctx, newHistory)
	if err != nil {
		return xerrors.Errorf("couldn't broadcast: %v", err)
	}

	// 3. Get the best history from the received messages.
	Bp, err := c.historiesFactory.FromMessageSet(prepareSet.GetBroadcasted())
	if err != nil {
		return encoding.NewDecodingError("broadcasted set", err)
	}

	// 4. Broadcast what we received in step 3.
	commitSet, err := c.broadcast.send(ctx, Bp.getBest())
	if err != nil {
		return xerrors.Errorf("couldn't broadcast: %v", err)
	}

	// 5. Get the best history from the second broadcast.
	Rpp, err := c.historiesFactory.FromMessageSet(commitSet.GetReceived())
	if err != nil {
		return encoding.NewDecodingError("received set", err)
	}
	c.history = Rpp.getBest()

	// 6. Verify that the best history is present and unique.
	broadcasted, err := c.historiesFactory.FromMessageSet(commitSet.GetBroadcasted())
	if err != nil {
		return encoding.NewDecodingError("broadcasted set", err)
	}
	received, err := c.historiesFactory.FromMessageSet(prepareSet.GetReceived())
	if err != nil {
		return encoding.NewDecodingError("received set", err)
	}

	if broadcasted.contains(c.history) && received.isUniqueBest(c.history) {
		// TODO: node responsible for the best proposal should broadcast
		// it to the others.
		last, ok := c.history.getLast()
		if ok {
			err := val.Commit(last.hash)
			if err != nil {
				return xerrors.Errorf("couldn't commit: %v", err)
			}
		}
	}

	return nil
}

// actor provides the primitive to send proposal to the consensus group.
//
// - implements consensus.Actor
type actor struct {
	ch      chan consensus.Proposal
	closing chan struct{}
}

// Propose implements consensus.Actor. It sends the proposal to the qsc loop. If
// the actor has been closed, it will panic.
func (a actor) Propose(proposal consensus.Proposal, players mino.Players) error {
	a.ch <- proposal
	return nil
}

// Close implements consensus.Actor. It stops and cleans the main loop.
func (a actor) Close() error {
	close(a.closing)

	return nil
}