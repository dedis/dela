package gossip

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

func TestFlat_Listen(t *testing.T) {
	gossiper := NewFlat(fake.Mino{}, nil)

	actor, err := gossiper.Listen()
	require.NoError(t, err)
	require.NotNil(t, actor)

	gossiper.mino = fake.NewBadMino()
	_, err = gossiper.Listen()
	require.EqualError(t, err, "couldn't create the rpc: fake error")
}

func TestFlat_Rumors(t *testing.T) {
	gossiper := NewFlat(nil, nil)
	require.NotNil(t, gossiper.Rumors())
}

func TestActor_SetPlayers(t *testing.T) {
	actor := &flatActor{}

	actor.SetPlayers(fake.NewAuthority(3, fake.NewSigner))
	require.Equal(t, 3, actor.players.Len())

	actor.SetPlayers(nil)
	require.Nil(t, actor.players)
}

func TestActor_Add(t *testing.T) {
	rpc := fake.NewRPC()
	actor := &flatActor{
		rpc:     rpc,
		players: fake.NewAuthority(3, fake.NewSigner),
	}

	close(rpc.Msgs)

	err := actor.Add(fakeRumor{})
	require.NoError(t, err)

	rpc = fake.NewRPC()
	actor.rpc = rpc
	rpc.Errs <- xerrors.New("oops")
	err = actor.Add(fakeRumor{})
	require.EqualError(t, err, "couldn't send the rumor: oops")

	actor.players = nil
	err = actor.Add(fakeRumor{})
	require.NoError(t, err)
}

func TestActor_Close(t *testing.T) {
	actor := &flatActor{
		players: fake.NewAuthority(3, fake.NewSigner),
	}

	require.NoError(t, actor.Close())
	require.Nil(t, actor.players)
}

func TestHandler_Process(t *testing.T) {
	h := handler{
		Flat: &Flat{
			rumorFactory: fakeRumorFactory{},
			ch:           make(chan Rumor, 1),
		},
	}

	resp, err := h.Process(mino.Request{Message: tmp.ProtoOf(fakeRumor{})})
	require.NoError(t, err)
	require.Nil(t, resp)

	h.rumorFactory = fake.MessageFactory{}
	_, err = h.Process(mino.Request{Message: tmp.ProtoOf(fake.Message{})})
	require.EqualError(t, err, "unexpected rumor of type 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeRumorFactory struct {
	serde.UnimplementedFactory
}

func (r fakeRumorFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	return fakeRumor{}, nil
}

type fakeRumor struct {
	serde.UnimplementedMessage
}

func (r fakeRumor) GetID() []byte {
	return []byte{0xa}
}

func (r fakeRumor) VisitJSON(serde.Serializer) (interface{}, error) {
	return struct{}{}, nil
}
