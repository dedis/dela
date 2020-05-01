package darc

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

func TestAction_Pack(t *testing.T) {
	action := darcAction{
		key:    []byte{0x01},
		access: NewAccess(),
	}

	pb, err := action.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*ActionProto)(nil), pb)

	actionpb := pb.(*ActionProto)
	require.Equal(t, action.key, actionpb.GetKey())

	_, err = action.Pack(fake.BadPackEncoder{})
	require.EqualError(t, err, "couldn't pack access: fake error")
}

func TestAction_Fingerprint(t *testing.T) {
	action := darcAction{
		key: []byte{0x01},
		access: Access{rules: map[string]expression{
			"\x02": {matches: map[string]struct{}{"\x03": {}}},
		}},
	}

	buffer := new(bytes.Buffer)

	err := action.Fingerprint(buffer, encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, "\x01\x02\x03", buffer.String())

	err = action.Fingerprint(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write key: fake error")

	err = action.Fingerprint(fake.NewBadHashWithDelay(1), nil)
	require.EqualError(t, err,
		"couldn't fingerprint access: couldn't write key: fake error")
}

func TestServerAction_Consume(t *testing.T) {
	action := serverAction{
		encoder:    encoding.NewProtoEncoder(),
		darcAction: darcAction{key: []byte{0x01}},
	}

	call := &fake.Call{}
	err := action.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 1, call.Len())
	// Key is provided so it's an update.
	require.Equal(t, []byte{0x01}, call.Get(0, 0))

	// No key thus it's a creation.
	action.darcAction.key = nil
	err = action.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())
	require.Equal(t, []byte{0x34}, call.Get(1, 0))

	action.encoder = fake.BadPackEncoder{}
	err = action.Consume(fakeContext{}, fakePage{})
	require.EqualError(t, err, "couldn't pack access: fake error")

	action.encoder = encoding.NewProtoEncoder()
	err = action.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write access: oops")
}

func TestActionFactory_FromProto(t *testing.T) {
	factory := NewActionFactory().(actionFactory)

	actionpb := &ActionProto{Key: []byte{0x02}, Access: &AccessProto{}}
	action, err := factory.FromProto(actionpb)
	require.NoError(t, err)
	require.IsType(t, serverAction{}, action)

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	factory.darcFactory = badArcFactory{}
	_, err = factory.FromProto(&ActionProto{})
	require.EqualError(t, err, "couldn't decode access: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeContext struct {
	basic.Context
}

func (ctx fakeContext) GetID() []byte {
	return []byte{0x34}
}

type fakePage struct {
	inventory.WritablePage
	call *fake.Call
	err  error
}

func (page fakePage) Write(key []byte, value proto.Message) error {
	page.call.Add(key, value)

	return page.err
}

type badArcFactory struct {
	arc.AccessControlFactory
}

func (f badArcFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return nil, xerrors.New("oops")
}
