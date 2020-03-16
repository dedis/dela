// Package qsc implements the Que Sera Consensus algorithm.
package qsc

import (
	"math/rand"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/mino"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Consensus is an abstraction to send proposals to a network of nodes that will
// decide to include them in the common state.
type Consensus struct {
	ch        chan consensus.Proposal
	history   history
	broadcast broadcast
}

// NewQSC returns a new instance of QSC.
func NewQSC(node int64, mino mino.Mino, players mino.Players) (*Consensus, error) {
	bc, err := newBroadcast(node, mino, players)
	if err != nil {
		return nil, err
	}

	return &Consensus{
		ch:        make(chan consensus.Proposal),
		history:   make(history, 0),
		broadcast: bc,
	}, nil
}

// GetChainFactory returns the chain factory.
func (c *Consensus) GetChainFactory() consensus.ChainFactory {
	return nil
}

// GetChain returns the chain that can prove the integrity of the proposal with
// the given identifier.
func (c *Consensus) GetChain(id []byte) (consensus.Chain, error) {
	return nil, nil
}

// Listen returns the actor that provides the primitives to send proposals to a
// network of nodes.
func (c *Consensus) Listen(val consensus.Validator) (consensus.Actor, error) {
	go func() {
		for {
			// 1. Choose the message and the random value.
			e := epoch{
				random: rand.Int63(),
			}

			select {
			case prop, ok := <-c.ch:
				if !ok {
					fabric.Logger.Trace().Msg("closing")
					return
				}

				e.hash = prop.GetHash()
			default:
				// If the current node does not have anything to propose, it
				// still has to participate so it sends an empty proposal.
				e.hash = nil
			}

			Hp := make(history, len(c.history), len(c.history)+1)
			copy(Hp, c.history)
			Hp = append(Hp, e)

			// 2. Broadcast our history to the network and get back messages
			// from this time step.
			view, err := c.broadcast.send(Hp)
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			// 3. Get the best history from the received messages.
			Bp, err := fromMessageSet(view.GetBroadcasted())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			// 4. Broadcast what we received in step 3.
			viewPrime, err := c.broadcast.send(Bp.getBest())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			// 5. Get the best history from the second broadcast.
			Rpp, err := fromMessageSet(viewPrime.GetReceived())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}
			c.history = Rpp.getBest()

			// 6. Verify that the best history is present and unique.
			broadcasted, err := fromMessageSet(viewPrime.GetBroadcasted())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}
			received, err := fromMessageSet(view.GetReceived())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			if broadcasted.contains(c.history) && received.isUniqueBest(c.history) {
				// TODO: node responsible for the best proposal should broadcast
				// it to the others.
				last, ok := c.history.getLast()
				if ok {
					val.Commit(last.hash)
				}
			}
		}
	}()

	return actor{ch: c.ch}, nil
}

type actor struct {
	ch chan consensus.Proposal
}

func (a actor) Propose(proposal consensus.Proposal, players mino.Players) error {
	a.ch <- proposal
	return nil
}
