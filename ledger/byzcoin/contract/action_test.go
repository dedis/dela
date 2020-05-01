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
		Argument:   &empty.Empty{},
	}

	pb, err := action.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*SpawnActionProto)(nil), pb)

	actionpb := pb.(*SpawnActionProto)
	require.Equal(t, action.ContractID, actionpb.GetContractID())
	require.True(t, ptypes.Is(actionpb.GetArgument(), action.Argument))

	_, err = action.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't pack argument: fake error")
}

func TestSpawnAction_Fingerprint(t *testing.T) {
	action := SpawnAction{
		ContractID: "deadbeef",
		Argument:   &empty.Empty{},
	}

	buffer := new(bytes.Buffer)
	encoder := encoding.NewProtoEncoder()

	err := action.Fingerprint(buffer, encoder)
	require.NoError(t, err)
	require.Equal(t, "deadbeef{}", buffer.String())

	err = action.Fingerprint(fake.NewBadHash(), encoder)
	require.EqualError(t, err, "couldn't write contract: fake error")

	err = action.Fingerprint(buffer, fake.BadMarshalStableEncoder{})
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestInvokeAction_Pack(t *testing.T) {
	action := InvokeAction{
		Key:      []byte{0x01},
		Argument: &empty.Empty{},
	}

	pb, err := action.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*InvokeActionProto)(nil), pb)

	actionpb := pb.(*InvokeActionProto)
	require.Equal(t, action.Key, actionpb.GetKey())
	require.True(t, ptypes.Is(actionpb.GetArgument(), action.Argument))

	_, err = action.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't pack argument: fake error")
}

func TestInvokeAction_WriteTo(t *testing.T) {
	action := InvokeAction{
		Key:      []byte{0x01},
		Argument: &empty.Empty{},
	}

	buffer := new(bytes.Buffer)
	encoder := encoding.NewProtoEncoder()

	err := action.Fingerprint(buffer, encoder)
	require.NoError(t, err)
	require.Equal(t, "\x01{}", buffer.String())

	err = action.Fingerprint(fake.NewBadHash(), encoder)
	require.EqualError(t, err, "couldn't write key: fake error")

	err = action.Fingerprint(buffer, fake.BadMarshalStableEncoder{})
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
	encoder := encoding.NewProtoEncoder()

	err := action.Fingerprint(buffer, encoder)
	require.NoError(t, err)
	require.Equal(t, "\x01", buffer.String())

	err = action.Fingerprint(fake.NewBadHash(), encoder)
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

	err = action.Consume(fakeContext{id: []byte("a")}, page)
	require.EqualError(t, err, "instance already exists")

	action.ClientAction = SpawnAction{ContractID: "unknown"}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	action.ClientAction = SpawnAction{ContractID: "bad"}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't execute spawn: oops")

	action.ClientAction = SpawnAction{ContractID: "fake"}
	factory.err = xerrors.New("oops")
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: couldn't read access: couldn't decode access: oops")

	factory.err = nil
	action.encoder = fake.BadMarshalAnyEncoder{}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't pack value: fake error")

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
		"couldn't read the instance: couldn't read the value: not found")

	action.ClientAction = InvokeAction{Key: []byte("z")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	action.ClientAction = InvokeAction{Key: []byte("b")}
	factory.err = xerrors.New("oops")
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: couldn't read access: couldn't decode access: oops")

	factory.err = nil
	factory.access.match = false
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: fake.PublicKey is refused to 'fake:invoke' by fakeAccessControl: not authorized")

	factory.access.match = true
	action.ClientAction = InvokeAction{Key: []byte("y")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't invoke: oops")

	action.ClientAction = InvokeAction{Key: []byte("a")}
	action.encoder = fake.BadMarshalAnyEncoder{}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't pack value: fake error")

	// 3. Consume a delete transaction.
	action.ClientAction = DeleteAction{Key: []byte("a")}

	err = action.Consume(fakeContext{}, page)
	require.NoError(t, err)

	action.ClientAction = DeleteAction{Key: []byte("c")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: couldn't read the value: not found")

	// 4. Consume an invalid transaction.
	page.err = xerrors.New("oops")
	action.ClientAction = DeleteAction{Key: []byte("a")}
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't write instance to page: oops")

	action.ClientAction = nil
	err = action.Consume(fakeContext{}, page)
	require.EqualError(t, err, "invalid action type '<nil>'")
}

func TestActionFactory_Register(t *testing.T) {
	factory := NewActionFactory()

	factory.Register("a", fakeContract{})
	factory.Register("b", fakeContract{})
	require.Len(t, factory.contracts, 2)

	factory.Register("a", fakeContract{})
	require.Len(t, factory.contracts, 2)
}

func TestActionFactory_FromProto(t *testing.T) {
	factory := NewActionFactory()

	spawnpb := &SpawnActionProto{ContractID: "A"}
	action, err := factory.FromProto(spawnpb)
	require.NoError(t, err)
	require.NotNil(t, action)

	spawnAny, err := ptypes.MarshalAny(spawnpb)
	require.NoError(t, err)
	action, err = factory.FromProto(spawnAny)
	require.NoError(t, err)
	require.NotNil(t, action)

	invokepb := &InvokeActionProto{Key: []byte{0x01}}
	action, err = factory.FromProto(invokepb)
	require.NoError(t, err)
	require.NotNil(t, action)

	deletepb := &DeleteActionProto{Key: []byte{0x01}}
	action, err = factory.FromProto(deletepb)
	require.NoError(t, err)
	require.NotNil(t, action)

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(spawnAny)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")
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

	return instance, nil
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
