package gossip

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&RumorProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestFlat_GetRumorFactory(t *testing.T) {
	gossiper := NewFlat(nil, fakeRumorFactory{})
	require.NotNil(t, gossiper.GetRumorFactory())
}

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
		encoder: encoding.NewProtoEncoder(),
		rpc:     rpc,
		players: fake.NewAuthority(3, fake.NewSigner),
	}

	close(rpc.Msgs)

	err := actor.Add(fakeRumor{})
	require.NoError(t, err)

	actor.encoder = fake.BadPackAnyEncoder{}
	err = actor.Add(fakeRumor{})
	require.EqualError(t, err, "couldn't pack rumor: fake error")

	actor.encoder = encoding.NewProtoEncoder()
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
		encoder: encoding.NewProtoEncoder(),
	}

	emptyAny, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	msg := &RumorProto{Message: emptyAny}

	resp, err := h.Process(mino.Request{Message: msg})
	require.NoError(t, err)
	require.Nil(t, resp)

	_, err = h.Process(mino.Request{})
	require.EqualError(t, err, "invalid message type '<nil>'")

	h.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = h.Process(mino.Request{Message: msg})
	require.EqualError(t, err, "couldn't pack rumor: fake error")

	h.encoder = encoding.NewProtoEncoder()
	h.rumorFactory = fakeRumorFactory{err: xerrors.New("oops")}
	_, err = h.Process(mino.Request{Message: msg})
	require.EqualError(t, err, "couldn't decode rumor: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeRumorFactory struct {
	RumorFactory
	err error
}

func (f fakeRumorFactory) FromProto(proto.Message) (Rumor, error) {
	return fakeRumor{}, f.err
}

type fakeRumor struct {
	Rumor
}

func (r fakeRumor) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, nil
}
