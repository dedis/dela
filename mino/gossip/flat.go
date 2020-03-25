package gossip

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Flat is an implementation of a message passing protocol that is using a flat
// communication approach.
type Flat struct {
	mino    mino.Mino
	players mino.Players
	decoder Decoder
	ch      chan Rumor
	rpc     mino.RPC
}

// NewFlat creates a new instance of a flat gossip protocol.
func NewFlat(m mino.Mino, dec Decoder) *Flat {
	return &Flat{
		mino:    m,
		decoder: dec,
		ch:      make(chan Rumor, 100),
	}
}

// Start creates the RPC and starts to listen for incoming rumors while
// spreading its own ones.
func (flat *Flat) Start(players mino.Players) error {
	rpc, err := flat.mino.MakeRPC("flatgossip", handler{Flat: flat})
	if err != nil {
		return xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	flat.rpc = rpc
	flat.players = players

	return nil
}

// Stop stops the gossiper.
func (flat *Flat) Stop() error {
	flat.rpc = nil

	return nil
}

// Add adds the rumor to the pool of rumors. It will be spread to the players.
func (flat *Flat) Add(rumor Rumor) error {
	if flat.rpc == nil {
		return xerrors.New("gossiper not started")
	}

	packed, err := rumor.Pack()
	if err != nil {
		return encoding.NewEncodingError("rumor", err)
	}

	packedAny, err := ptypes.MarshalAny(packed)
	if err != nil {
		return encoding.NewAnyEncodingError(packed, err)
	}

	req := &RumorProto{Message: packedAny}

	resps, errs := flat.rpc.Call(req, flat.players)
	for {
		select {
		case _, ok := <-resps:
			if !ok {
				return nil
			}
		case err := <-errs:
			return err
		}
	}
}

// Rumors returns the channel that is populated with new rumors.
func (flat *Flat) Rumors() <-chan Rumor {
	return flat.ch
}

type handler struct {
	*Flat
	mino.UnsupportedHandler
}

func (h handler) Process(pb proto.Message) (proto.Message, error) {
	switch msg := pb.(type) {
	case *RumorProto:
		dynamicAny := &ptypes.DynamicAny{}
		err := ptypes.UnmarshalAny(msg.GetMessage(), dynamicAny)
		if err != nil {
			return nil, encoding.NewAnyDecodingError(msg.GetMessage(), err)
		}

		rumor, err := h.decoder(dynamicAny.Message)
		if err != nil {
			return nil, encoding.NewDecodingError("rumor", err)
		}

		h.ch <- rumor

		return nil, nil
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", pb)
	}
}
