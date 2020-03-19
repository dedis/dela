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
	history          history
	broadcast        broadcast
	historiesFactory historiesFactory

	Stopped chan struct{}
}

// NewQSC returns a new instance of QSC.
func NewQSC(node int64, mino mino.Mino, players mino.Players) (*Consensus, error) {
	bc, err := newBroadcast(node, mino, players)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create broadcast: %v", err)
	}

	return &Consensus{
		ch:               make(chan consensus.Proposal),
		history:          make(history, 0),
		broadcast:        bc,
		historiesFactory: defaultHistoriesFactory{},
		Stopped:          make(chan struct{}),
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
			case prop, ok := <-c.ch:
				if !ok {
					fabric.Logger.Trace().Msg("closing")
					return
				}

				proposal = prop
			default:
				// If the current node does not have anything to propose, it
				// still has to participate so it sends an empty proposal.
				proposal = nil
			}

			err := c.executeRound(proposal, val)
			if err != nil {
				fabric.Logger.Err(err).Msg("failed to execute a time step")
			}
		}
	}()

	return actor{ch: c.ch}, nil
}

// Close stops and cleans the main loop.
func (c *Consensus) Close() error {
	close(c.ch)

	return nil
}

func (c *Consensus) executeRound(prop consensus.Proposal, val consensus.Validator) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), EpochTimeout)
	defer cancel()

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
	ch chan consensus.Proposal
}

// Propose implements consensus.Actor. It sends the proposal to the qsc loop.
func (a actor) Propose(proposal consensus.Proposal, players mino.Players) error {
	a.ch <- proposal
	return nil
}
