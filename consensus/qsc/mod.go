// Package qsc implements the Que Sera Consensus algorithm.
package qsc

import (
	"math/rand"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

type epoch struct {
	consensus.Proposal
	random int
}

// Consensus is an abstraction to send proposals to a network of nodes that will
// decide to include them in the common state.
type Consensus struct {
	node      int
	ch        chan consensus.Proposal
	history   []epoch
	broadcast broadcast
}

// GetChainFactory returns the chain factory.
func (c Consensus) GetChainFactory() consensus.ChainFactory {
	return nil
}

// GetChain returns the chain that can prove the integrity of the proposal with
// the given identifier.
func (c Consensus) GetChain(id []byte) (consensus.Chain, error) {
	return nil, nil
}

// Listen returns the actor that provides the primitives to send proposals to a
// network of nodes.
func (c Consensus) Listen(val consensus.Validator) (consensus.Actor, error) {
	go func() {
		for {
			// ChooseMessage + RandomValue
			e := epoch{
				random: rand.Int(),
			}

			select {
			case e.Proposal = <-c.ch:
			default:
				e.Proposal = emptyProposal{}
			}

			// Create the node proposed history for the round.
			Hp := make([]epoch, len(c.history), len(c.history)+1)
			copy(Hp, c.history)
			Hp = append(Hp, e)

			// Broadcast.. !
			Rp, Bp, err := c.broadcast.send(nil)
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			// Get the best history from the broadcast.
			Hpp := Bp.getBest()

			Rpp, Bpp, err := c.broadcast.send(Hpp[0])
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			c.history = Rpp.getBest()

			if Bpp.contains(c.history) && Rp.isUniqueBest(c.history) {
				// TODO: deliver to the upper layer
				val.Commit(nil)
			}
		}
	}()

	return nil, nil
}

type actor struct{}

func (a actor) Propose(proposal consensus.Proposal, players mino.Players) error {
	return nil
}

type view struct{}

func (v view) getBest() []epoch {
	return []epoch{}
}

func (v view) isUniqueBest(h []epoch) bool {
	return false
}

func (v view) contains(h []epoch) bool {
	return false
}

type emptyProposal struct{}

func (p emptyProposal) GetHash() []byte {
	return nil
}

func (p emptyProposal) GetPreviousHash() []byte {
	return nil
}

func (p emptyProposal) GetVerifier() crypto.Verifier {
	return nil
}

func (p emptyProposal) Pack() (proto.Message, error) {
	return nil, nil
}
