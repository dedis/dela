package gossip

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

const (
	rumorTimeout = 10 * time.Second
)

// Flat is an implementation of a message passing protocol that is using a flat
// communication approach.
//
// - implements gossip.Gossiper
type Flat struct {
	sync.RWMutex
	mino         mino.Mino
	rumorFactory serde.Factory
	ch           chan Rumor
}

// NewFlat creates a new instance of a flat gossip protocol.
func NewFlat(m mino.Mino, f serde.Factory) *Flat {
	return &Flat{
		mino:         m,
		rumorFactory: f,
		ch:           make(chan Rumor, 100),
	}
}

// Listen implements gossip.Gossiper. It creates the RPC and starts to listen
// for incoming rumors while spreading its own ones.
func (flat *Flat) Listen() (Actor, error) {
	h := handler{Flat: flat}

	rpc, err := flat.mino.MakeRPC("flatgossip", h)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	actor := &flatActor{
		rpc: rpc,
	}

	return actor, nil
}

// Rumors implements gossip.Gossiper. It returns the channel that is populated
// with new rumors.
func (flat *Flat) Rumors() <-chan Rumor {
	return flat.ch
}

type flatActor struct {
	sync.Mutex

	rpc     mino.RPC
	players mino.Players
}

// SetPlayers implements gossip.Actor. It changes the set of participants where
// the rumors will be sent.
func (a *flatActor) SetPlayers(players mino.Players) {
	a.Lock()
	a.players = players
	a.Unlock()
}

// Add implements gossip.Actor. It adds the rumor to the pool of rumors. It will
// be spread to the players.
func (a *flatActor) Add(rumor Rumor) error {
	a.Lock()
	players := a.players
	a.Unlock()

	if players == nil {
		// Drop rumors if the network is empty.
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), rumorTimeout)
	defer cancel()

	resps, errs := a.rpc.Call(ctx, tmp.ProtoOf(rumor), players)
	for {
		select {
		case _, more := <-resps:
			if !more {
				return nil
			}
		case err := <-errs:
			return xerrors.Errorf("couldn't send the rumor: %v", err)
		}
	}
}

// Close implements gossip.Actor. It stops the gossip actor.
func (a *flatActor) Close() error {
	a.Lock()
	a.players = nil
	a.Unlock()

	return nil
}

// Handler processes the messages coming from the gossip network.
//
// - implements mino.Handler
type handler struct {
	*Flat
	mino.UnsupportedHandler
}

// Process implements mino.Handler. It notifies the new rumor if appropriate and
// does not return anything.
func (h handler) Process(req mino.Request) (proto.Message, error) {
	m := tmp.FromProto(req.Message, h.rumorFactory)

	rumor, ok := m.(Rumor)
	if !ok {
		return nil, xerrors.Errorf("unexpected rumor of type '%T'", m)
	}

	h.ch <- rumor

	return nil, nil
}
