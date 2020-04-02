package smartcontract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/permissions"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&InstanceProto{},
		&TransactionProto{},
		&Spawn{},
		&Invoke{},
		&Delete{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestConsumer_Register(t *testing.T) {
	c := NewConsumer()

	c.Register("contract", fakeContract{})
	require.Len(t, c.contracts, 1)

	c.Register("another contract", fakeContract{})
	require.Len(t, c.contracts, 2)

	c.Register("contract", fakeContract{})
	require.Len(t, c.contracts, 2)
}

func TestConsumer_GetTransactionFactory(t *testing.T) {
	c := NewConsumer()
	require.NotNil(t, c.GetTransactionFactory())
}

func TestConsumer_GetInstanceFactory(t *testing.T) {
	c := NewConsumer()
	require.NotNil(t, c.GetInstanceFactory())
}

func TestConsumer_Consume(t *testing.T) {
	factory := &fakeAccessFactory{access: &fakeAccessControl{match: true}}

	c := NewConsumer()
	c.AccessFactory = factory
	c.Register("fake", fakeContract{})
	c.Register("bad", fakeContract{err: xerrors.New("oops")})

	// 1. Consume a spawn transaction.
	tx := transaction{
		hash:     []byte{0xab},
		identity: fakeIdentity{},
		action: SpawnAction{
			ContractID: "fake",
		},
	}

	out, err := c.Consume(newContext(tx, nil))
	require.NoError(t, err)
	require.Equal(t, tx.hash, out.GetKey())

	tx.action = SpawnAction{ContractID: "abc"}
	_, err = c.Consume(newContext(tx, nil))
	require.EqualError(t, err, "unknown contract with id 'abc'")

	tx.action = SpawnAction{ContractID: "bad"}
	_, err = c.Consume(newContext(tx, nil))
	require.EqualError(t, err, "couldn't execute spawn: oops")

	// 2. Consume an invoke transaction.
	c.encoder = encoding.NewProtoEncoder()
	tx.action = InvokeAction{
		Key:      []byte{0xab},
		Argument: &empty.Empty{},
	}

	instance := makeInstance()
	instance.key = []byte{0xab}
	ctx := newContext(tx, instance)
	out, err = c.Consume(ctx)
	require.NoError(t, err)
	require.Equal(t, []byte{0xab}, out.GetKey())
	require.Len(t, factory.access.calls, 1)
	require.Equal(t, fakeIdentity{}, factory.access.calls[0][0])
	require.Equal(t, "invoke:fake", factory.access.calls[0][1])

	_, err = c.Consume(testContext{tx: tx, errRead: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read the instance: oops")

	instance.contractID = "unknown"
	_, err = c.Consume(newContext(tx, instance))
	require.EqualError(t, err, "unknown contract with id 'unknown'")

	instance.contractID = "fake"
	factory.err = xerrors.New("oops")
	_, err = c.Consume(ctx)
	require.EqualError(t, err, "couldn't read access control: oops")

	factory.err = nil
	factory.access.match = false
	_, err = c.Consume(ctx)
	require.EqualError(t, err, "fakePublicKey is refused to 'invoke:fake' by fakeAccessControl")

	factory.access.match = true
	instance.contractID = "bad"
	_, err = c.Consume(newContext(tx, instance))
	require.EqualError(t, err, "couldn't invoke: oops")

	// 3. Consume a delete transaction.
	tx.action = DeleteAction{
		Key: []byte{0xab},
	}

	out, err = c.Consume(newContext(tx, makeInstance()))
	require.NoError(t, err)
	require.True(t, out.(contractInstance).deleted)

	_, err = c.Consume(testContext{tx: tx, errRead: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read the instance: oops")

	// 4. Consume an invalid transaction.
	_, err = c.Consume(newContext(fakeTx{}, nil))
	require.EqualError(t, err, "invalid tx type 'smartcontract.fakeTx'")

	tx.action = nil
	_, err = c.Consume(newContext(tx, nil))
	require.EqualError(t, err, "invalid action type '<nil>'")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstance() contractInstance {
	return contractInstance{
		value:      &empty.Empty{},
		contractID: "fake",
		deleted:    false,
	}
}

type fakeContract struct {
	Contract
	err error
}

func (c fakeContract) Spawn(ctx SpawnContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, c.err
}

func (c fakeContract) Invoke(ctx InvokeContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, c.err
}

type fakeTx struct {
	consumer.Transaction
}

type fakeAccessControl struct {
	permissions.AccessControl
	match bool
	calls [][]interface{}
}

func (ac *fakeAccessControl) Match(ident permissions.Identity, rule string) bool {
	ac.calls = append(ac.calls, []interface{}{ident, rule})
	return ac.match
}

func (ac *fakeAccessControl) String() string {
	return "fakeAccessControl"
}

type testContext struct {
	tx       consumer.Transaction
	instance ContractInstance
	errRead  error
}

func newContext(tx consumer.Transaction, inst ContractInstance) testContext {
	return testContext{
		tx:       tx,
		instance: inst,
	}
}

func (c testContext) GetTransaction() consumer.Transaction {
	return c.tx
}

func (c testContext) Read([]byte) (consumer.Instance, error) {
	return c.instance, c.errRead
}

type fakeAccessFactory struct {
	permissions.AccessControlFactory
	access *fakeAccessControl
	err    error
}

func (f *fakeAccessFactory) FromProto(proto.Message) (permissions.AccessControl, error) {
	return f.access, f.err
}
