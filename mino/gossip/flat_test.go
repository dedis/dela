package gossip

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func TestFlat_Listen(t *testing.T) {
	gossiper := NewFlat(fake.Mino{}, nil)

	actor, err := gossiper.Listen()
	require.NoError(t, err)
	require.NotNil(t, actor)
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

	rpc.Done()
	err := actor.Add(fakeRumor{})
	require.NoError(t, err)

	actor.rpc = fake.NewBadRPC()
	err = actor.Add(fakeRumor{})
	require.EqualError(t, err, fake.Err("couldn't call peers"))

	buffer := new(bytes.Buffer)
	rpc = fake.NewRPC()
	actor.rpc = rpc
	actor.logger = zerolog.New(buffer).Level(zerolog.WarnLevel)
	rpc.SendResponseWithError(nil, fake.GetError())
	rpc.Done()

	err = actor.Add(fakeRumor{})
	require.NoError(t, err)
	require.Contains(t, buffer.String(), `"message":"rumor not sent"`)

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

	resp, err := h.Process(mino.Request{Message: fakeRumor{}})
	require.NoError(t, err)
	require.Nil(t, resp)

	h.rumorFactory = fake.MessageFactory{}
	_, err = h.Process(mino.Request{Message: fake.Message{}})
	require.EqualError(t, err, "unexpected rumor of type 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeRumorFactory struct{}

func (r fakeRumorFactory) Deserialize(serde.Context, []byte) (serde.Message, error) {
	return fakeRumor{}, nil
}

type fakeRumor struct{}

func (r fakeRumor) GetID() []byte {
	return []byte{0xa}
}

func (r fakeRumor) Serialize(serde.Context) ([]byte, error) {
	return []byte("{}"), nil
}
