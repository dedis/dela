//
// Documentation Last Review: 06.10.2020
//

package gossip

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const (
	rumorTimeout = 10 * time.Second
)

// Flat is an implementation of a message passing protocol that is using a flat
// communication approach by sending a rumor to all the known participants.
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

	actor := &flatActor{
		logger: dela.Logger.With().Str("addr", flat.mino.GetAddress().String()).Logger(),
		rpc:    mino.MustCreateRPC(flat.mino, "flatgossip", h, flat.rumorFactory),
	}

	return actor, nil
}

// Rumors implements gossip.Gossiper. It returns the channel that is populated
// with new rumors.
func (flat *Flat) Rumors() <-chan Rumor {
	return flat.ch
}

// flatActor is the actor returned by the gossiper that provide the primitives
// to send a rumor.
//
// - implements gossip.Actor
type flatActor struct {
	sync.Mutex

	logger  zerolog.Logger
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

	resps, err := a.rpc.Call(ctx, rumor, players)
	if err != nil {
		return xerrors.Errorf("couldn't call peers: %v", err)
	}

	for {
		resp, more := <-resps
		if !more {
			return nil
		}

		_, err := resp.GetMessageOrError()
		if err != nil {
			a.logger.Warn().Err(err).Msg("rumor not sent")
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
func (h handler) Process(req mino.Request) (serde.Message, error) {
	rumor, ok := req.Message.(Rumor)
	if !ok {
		return nil, xerrors.Errorf("unexpected rumor of type '%T'", req.Message)
	}

	h.ch <- rumor

	return nil, nil
}
