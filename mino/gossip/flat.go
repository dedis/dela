package gossip

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
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
	rumorFactory RumorFactory
	ch           chan Rumor
	encoder      encoding.ProtoMarshaler
}

// NewFlat creates a new instance of a flat gossip protocol.
func NewFlat(m mino.Mino, f RumorFactory) *Flat {
	return &Flat{
		mino:         m,
		rumorFactory: f,
		encoder:      encoding.NewProtoEncoder(),
		ch:           make(chan Rumor, 100),
	}
}

// GetRumorFactory implements gossip.Gossiper. It returns the rumor factory of
// the gossiper.
func (flat *Flat) GetRumorFactory() RumorFactory {
	return flat.rumorFactory
}

// Listen implements gossip.Gossiper. It creates the RPC and starts to listen
// for incoming rumors while spreading its own ones.
func (flat *Flat) Listen() (Actor, error) {
	h := handler{
		Flat:    flat,
		encoder: encoding.NewProtoEncoder(),
	}

	rpc, err := flat.mino.MakeRPC("flatgossip", h)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	actor := &flatActor{
		encoder: flat.encoder,
		rpc:     rpc,
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
	encoder encoding.ProtoMarshaler
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

	rumorpb, err := a.encoder.PackAny(rumor)
	if err != nil {
		return xerrors.Errorf("couldn't pack rumor: %v", err)
	}

	req := &RumorProto{Message: rumorpb}

	ctx, cancel := context.WithTimeout(context.Background(), rumorTimeout)
	defer cancel()

	resps, errs := a.rpc.Call(ctx, req, players)
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

type handler struct {
	*Flat
	mino.UnsupportedHandler

	encoder encoding.ProtoMarshaler
}

func (h handler) Process(req mino.Request) (proto.Message, error) {
	switch msg := req.Message.(type) {
	case *RumorProto:
		message, err := h.encoder.UnmarshalDynamicAny(msg.GetMessage())
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack rumor: %v", err)
		}

		rumor, err := h.rumorFactory.FromProto(message)
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode rumor: %v", err)
		}

		h.ch <- rumor

		return nil, nil
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", req.Message)
	}
}
