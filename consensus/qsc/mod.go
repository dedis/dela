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

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/qsc/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

const (
	// EpochTimeout is the maximum amount of time given to an epoch to end
	// before the request is aborted.
	EpochTimeout = 20 * time.Second
)

// Consensus is an abstraction to send proposals to a network of nodes that will
// decide to include them in the common state.
type Consensus struct {
	ch        chan serdeng.Message
	closing   chan struct{}
	stopped   chan struct{}
	history   types.History
	broadcast broadcast
}

// NewQSC returns a new instance of QSC.
func NewQSC(node int64, mino mino.Mino, players mino.Players) (*Consensus, error) {
	bc, err := newBroadcast(node, mino, players, types.HistoryFactory{})
	if err != nil {
		return nil, xerrors.Errorf("couldn't create broadcast: %v", err)
	}

	return &Consensus{
		ch:        make(chan serdeng.Message),
		closing:   make(chan struct{}),
		stopped:   make(chan struct{}),
		history:   types.History{},
		broadcast: bc,
	}, nil
}

// GetChainFactory implements consensus.Consensus. It returns the chain factory.
func (c *Consensus) GetChainFactory() serde.Factory {
	return nil
}

// GetChain implements consensus.Consensus. It returns the chain that can prove
// the integrity of the proposal with the given identifier.
func (c *Consensus) GetChain(id []byte) (consensus.Chain, error) {
	return nil, nil
}

// Listen implements consensus.Consensus. It returns the actor that provides the
// primitives to send proposals to a network of nodes.
func (c *Consensus) Listen(r consensus.Reactor) (consensus.Actor, error) {
	go func() {
		for {
			var proposal serdeng.Message
			select {
			case <-c.closing:
				dela.Logger.Trace().Msg("closing")
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

			err := c.executeRound(ctx, proposal, r)
			if err != nil {
				select {
				case <-c.closing:
				default:
					// Only log if the consensus has not been closed properly.
					dela.Logger.Err(err).Msg("failed to execute a time step")
				}
			}

			cancel()
		}
	}()

	return actor{ch: c.ch, closing: c.closing}, nil
}

func (c *Consensus) executeRound(
	ctx context.Context,
	prop serdeng.Message,
	val consensus.Reactor,
) error {
	// 1. Choose the message and the random value. The new epoch will be
	// appended to the current history.

	// TODO: ask about randomness
	num := rand.Int63()

	var hash []byte
	if prop != nil {
		// TODO: address
		digest, err := val.InvokeValidate(nil, prop)
		if err != nil {
			return xerrors.Errorf("couldn't validate proposal: %v", err)
		}

		hash = digest
	}

	newHistory := types.NewHistory(append(c.history.GetEpochs(), types.NewEpoch(hash, num))...)

	// 2. Broadcast our history to the network and get back messages
	// from this time step.
	prepareSet, err := c.broadcast.send(ctx, newHistory)
	if err != nil {
		return xerrors.Errorf("couldn't broadcast: %v", err)
	}

	// 3. Get the best history from the received messages.
	Bp := types.NewHistories(prepareSet.broadcasted)

	// 4. Broadcast what we received in step 3.
	commitSet, err := c.broadcast.send(ctx, Bp.GetBest())
	if err != nil {
		return xerrors.Errorf("couldn't broadcast: %v", err)
	}

	// 5. Get the best history from the second broadcast.
	Rpp := types.NewHistories(commitSet.received)
	c.history = Rpp.GetBest()

	// 6. Verify that the best history is present and unique.
	broadcasted := types.NewHistories(commitSet.broadcasted)
	received := types.NewHistories(prepareSet.received)

	if broadcasted.Contains(c.history) && received.IsUniqueBest(c.history) {
		// TODO: node responsible for the best proposal should broadcast
		// it to the others.
		last, ok := c.history.GetLast()
		if ok {
			err := val.InvokeCommit(last.GetHash())
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
	ch      chan serdeng.Message
	closing chan struct{}
}

// Propose implements consensus.Actor. It sends the proposal to the qsc loop. If
// the actor has been closed, it will panic.
func (a actor) Propose(proposal serdeng.Message) error {
	a.ch <- proposal
	return nil
}

// Close implements consensus.Actor. It stops and cleans the main loop.
func (a actor) Close() error {
	close(a.closing)

	return nil
}
