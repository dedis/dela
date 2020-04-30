package contract

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestSpawnAction_Pack(t *testing.T) {
	action := SpawnAction{
		ContractID: "deadbeef",
		Argument:   []byte{0x01},
	}

	pb, err := action.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*SpawnActionProto)(nil), pb)

	actionpb := pb.(*SpawnActionProto)
	require.Equal(t, action.ContractID, actionpb.GetContractID())
	require.Equal(t, action.Argument, actionpb.GetArgument())
}

func TestSpawnAction_WriteTo(t *testing.T) {
	action := SpawnAction{
		ContractID: "deadbeef",
		Argument:   []byte{0x01},
	}

	buffer := new(bytes.Buffer)
	sum, err := action.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, int64(9), sum)

	_, err = action.WriteTo(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write contract: fake error")

	_, err = action.WriteTo(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestInvokeAction_Pack(t *testing.T) {
	action := InvokeAction{
		Key:      []byte{0x1},
		Argument: []byte{0x02},
	}

	pb, err := action.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*InvokeActionProto)(nil), pb)

	actionpb := pb.(*InvokeActionProto)
	require.Equal(t, action.Key, actionpb.GetKey())
	require.Equal(t, action.Argument, actionpb.GetArgument())
}

func TestInvokeAction_WriteTo(t *testing.T) {
	action := InvokeAction{
		Key:      []byte{0x01},
		Argument: []byte{0x02},
	}

	buffer := new(bytes.Buffer)
	sum, err := action.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, int64(2), sum)

	_, err = action.WriteTo(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write key: fake error")

	_, err = action.WriteTo(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestDeleteAction_Pack(t *testing.T) {
	action := DeleteAction{
		Key: []byte{0x01},
	}

	pb, err := action.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*DeleteActionProto)(nil), pb)

	actionpb := pb.(*DeleteActionProto)
	require.Equal(t, action.Key, actionpb.GetKey())
}

func TestDeleteAction_WriteTo(t *testing.T) {
	action := DeleteAction{
		Key: []byte{0x01},
	}

	buffer := new(bytes.Buffer)

	sum, err := action.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, int64(1), sum)

	_, err = action.WriteTo(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write key: fake error")
}

func TestServerAction_Consume(t *testing.T) {
	factory := &fakeAccessFactory{access: &fakeAccess{match: true}}
	contracts := map[string]Contract{
		"fake": fakeContract{},
		"bad":  fakeContract{err: xerrors.New("oops")},
	}

	action := serverAction{
		ClientAction: SpawnAction{ContractID: "fake"},
		contracts:    contracts,
		arcFactory:   factory,
		encoder:      encoding.NewProtoEncoder(),
	}

	page := fakePage{
		store: map[string]proto.Message{
			"a":   makeInstance(t),
			"y":   &Instance{ContractID: "bad", AccessControl: []byte("arc")},
			"z":   &Instance{ContractID: "unknown", AccessControl: []byte("arc")},
			"arc": &empty.Empty{},
		},
	}

	// 1. Consume a spawn action.
	err := action.Consume(fakeContext{id: []byte("b")}, page)
	require.NoError(t, err)

	action.ClientAction = SpawnAction{ContractID: "bad"}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't execute spawn: oops")

	// 2. Consume an invoke transaction.
	action.encoder = encoding.NewProtoEncoder()
	action.ClientAction = InvokeAction{Key: []byte("b")}

	factory.access.calls = make([][]interface{}, 0)
	err = action.Consume(fakeContext{}, page)
	require.NoError(t, err)
	require.Len(t, factory.access.calls, 1)
	require.Equal(t, []arc.Identity{fake.PublicKey{}}, factory.access.calls[0][0])
	require.Equal(t, arc.Compile("fake", "invoke"), factory.access.calls[0][1])

	action.ClientAction = InvokeAction{Key: []byte("c")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: couldn't read the entry: not found")

	action.ClientAction = InvokeAction{Key: []byte("z")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	action.ClientAction = InvokeAction{Key: []byte("b")}
	factory.err = xerrors.New("oops")
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "no access: couldn't decode access: oops")

	factory.err = nil
	factory.access.match = false
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: fake.PublicKey is refused to 'fake:invoke' by fakeAccessControl: not authorized")

	factory.access.match = true
	action.ClientAction = InvokeAction{Key: []byte("y")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't invoke: oops")

	// 3. Consume a delete transaction.
	action.ClientAction = DeleteAction{Key: []byte("a")}

	err = action.Consume(fakeContext{}, page)
	require.NoError(t, err)

	action.ClientAction = DeleteAction{Key: []byte("c")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: couldn't read the entry: not found")

	// 4. Consume an invalid transaction.
	action.ClientAction = nil
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "missing action")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstance(t *testing.T) *Instance {
	value, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	return &Instance{
		ContractID:    "fake",
		AccessControl: []byte("arc"),
		Deleted:       false,
		Value:         value,
	}
}

type fakeContract struct {
	Contract
	err error
}

func (c fakeContract) Spawn(ctx SpawnContext) (proto.Message, []byte, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, []byte("arc"), c.err
}

func (c fakeContract) Invoke(ctx InvokeContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, c.err
}

type fakePage struct {
	inventory.WritablePage
	store map[string]proto.Message
	err   error
}

func (page fakePage) Read(key []byte) (proto.Message, error) {
	instance := page.store[string(key)]
	if instance == nil {
		return nil, xerrors.New("not found")
	}

	return instance, page.err
}

func (page fakePage) Write(key []byte, value proto.Message) error {
	page.store[string(key)] = value
	return page.err
}

type fakeContext struct {
	id []byte
}

func (ctx fakeContext) GetID() []byte {
	return ctx.id
}

func (ctx fakeContext) GetIdentity() arc.Identity {
	return fake.PublicKey{}
}

type fakeAccess struct {
	arc.AccessControl
	match bool
	calls [][]interface{}
}

func (ac *fakeAccess) Match(rule string, idents ...arc.Identity) error {
	ac.calls = append(ac.calls, []interface{}{idents, rule})
	if ac.match {
		return nil
	}
	return xerrors.New("not authorized")
}

func (ac *fakeAccess) String() string {
	return "fakeAccessControl"
}

type fakeAccessFactory struct {
	arc.AccessControlFactory
	access *fakeAccess
	err    error
}

func (f *fakeAccessFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return f.access, f.err
}
