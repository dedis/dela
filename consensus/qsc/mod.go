// Package qsc implements the Que Sera Consensus algorithm.
package qsc

import (
	"bytes"
	"math/rand"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
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
			// ChooseMessage + RandomValue
			e := epoch{
				random: rand.Int63(),
			}

			var ok bool
			select {
			case e.proposal, ok = <-c.ch:
				if !ok {
					fabric.Logger.Trace().Msg("closing")
					return
				}
			default:
				e.proposal = nil
			}

			// Create the node proposed history for the round.
			Hp := make(history, len(c.history), len(c.history)+1)
			copy(Hp, c.history)
			Hp = append(Hp, e)

			// Broadcast.. !
			view, err := c.broadcast.send(Hp)
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			// Get the best history from the broadcast.
			Bp, err := fromMessageSet(val, view.GetBroadcasted())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			viewPrime, err := c.broadcast.send(Bp.getBest())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			Rpp, err := fromMessageSet(val, viewPrime.GetReceived())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}
			c.history = Rpp.getBest()

			broadcasted, err := fromMessageSet(val, viewPrime.GetBroadcasted())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}
			received, err := fromMessageSet(val, view.GetReceived())
			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			if broadcasted.contains(c.history) && received.isUniqueBest(c.history) {
				// TODO: deliver to the upper layer
				val.Commit(nil)
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

type epoch struct {
	proposal consensus.Proposal
	random   int64
}

func (e epoch) Pack() (proto.Message, error) {
	pb := &Epoch{
		Random:   e.random,
		Proposal: nil, // TODO
	}

	return pb, nil
}

func (e epoch) Equal(other epoch) bool {
	if e.random != other.random {
		return false
	}
	if e.proposal != nil || other.proposal != nil {
		if e.proposal != nil && other.proposal != nil {
			if !bytes.Equal(e.proposal.GetHash(), other.proposal.GetHash()) {
				return false
			}
		} else {
			return false
		}
	}

	return true
}

type history []epoch

func (h history) getLast() epoch {
	return h[len(h)-1]
}

func (h history) Equal(other history) bool {
	if len(h) != len(other) {
		return false
	}

	for i, e := range h {
		if !e.Equal(other[i]) {
			return false
		}
	}

	return true
}

func (h history) Pack() (proto.Message, error) {
	pb := &History{
		Epochs: make([]*Epoch, len(h)),
	}

	for i, epoch := range h {
		packed, err := epoch.Pack()
		if err != nil {
			return nil, err
		}

		pb.Epochs[i] = packed.(*Epoch)
	}

	return pb, nil
}

type histories []history

func fromMessageSet(v consensus.Validator, ms map[int64]*Message) (histories, error) {
	hists := make(histories, len(ms))
	for i, msg := range ms {
		hist := &History{}
		err := ptypes.UnmarshalAny(msg.GetValue(), hist)
		if err != nil {
			return nil, err
		}

		epochs := make([]epoch, len(hist.GetEpochs()))
		for j, e := range hist.GetEpochs() {
			prop, err := v.Validate(e.GetProposal())
			if err != nil {
				return nil, err
			}

			epochs[j] = epoch{
				random:   e.GetRandom(),
				proposal: prop,
			}
		}

		hists[i] = history(epochs)
	}

	return hists, nil
}

func (hists histories) getBest() history {
	best := 0
	random := int64(0)
	for i, h := range hists[1:] {
		if h.getLast().random > random {
			random = h.getLast().random
			best = i
		}
	}

	return hists[best]
}

func (hists histories) contains(h history) bool {
	for _, history := range hists {
		if history.getLast().Equal(h.getLast()) {
			return true
		}
	}

	return false
}

func (hists histories) isUniqueBest(h history) bool {
	for _, history := range hists {
		isEqual := history.Equal(h)

		if !isEqual && history.getLast().random >= h.getLast().random {
			return false
		}
	}

	return true
}
