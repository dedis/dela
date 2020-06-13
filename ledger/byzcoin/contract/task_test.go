package contract

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestSpawnTask_Fingerprint(t *testing.T) {
	task := SpawnTask{
		ContractID: "deadbeef",
		Argument:   map[string]string{"A": "B"},
	}

	buffer := new(bytes.Buffer)

	err := task.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "deadbeefA:B", buffer.String())

	err = task.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write contract: fake error")

	err = task.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestInvokeTask_Fingerprint(t *testing.T) {
	task := InvokeTask{
		Key:      []byte{0x01},
		Argument: map[string]string{"A": "B"},
	}

	buffer := new(bytes.Buffer)

	err := task.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x01A:B", buffer.String())

	err = task.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write key: fake error")

	err = task.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestDeleteTask_Fingerprint(t *testing.T) {
	task := DeleteTask{
		Key: []byte{0x01},
	}

	buffer := new(bytes.Buffer)

	err := task.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x01", buffer.String())

	err = task.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write key: fake error")
}

func TestServerTask_Consume(t *testing.T) {
	contracts := map[string]Contract{
		"fake": fakeContract{},
		"bad":  fakeContract{err: xerrors.New("oops")},
	}

	task := serverTask{
		ClientTask: SpawnTask{ContractID: "fake"},
		contracts:  contracts,
	}

	page := fakePage{
		store: map[string]serde.Message{
			"a":   makeInstance(t),
			"y":   &Instance{ContractID: "bad", AccessControl: []byte("arc")},
			"z":   &Instance{ContractID: "unknown", AccessControl: []byte("arc")},
			"arc": &fakeAccess{match: true},
		},
	}

	// 1. Consume a spawn task.
	err := task.Consume(fakeContext{id: []byte("b")}, page)
	require.NoError(t, err)

	err = task.Consume(fakeContext{id: []byte("a")}, page)
	require.EqualError(t, err, "instance already exists")

	task.ClientTask = SpawnTask{ContractID: "unknown"}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	task.ClientTask = SpawnTask{ContractID: "bad"}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't execute spawn: oops")

	// 2. Consume an invoke task.
	task.ClientTask = InvokeTask{Key: []byte("b")}

	err = task.Consume(fakeContext{}, page)
	require.NoError(t, err)

	task.ClientTask = InvokeTask{Key: []byte("c")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: invalid message type '<nil>' != '*contract.Instance'")

	task.ClientTask = InvokeTask{Key: []byte("z")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	task.ClientTask = InvokeTask{Key: []byte("b")}
	page.store["arc"] = &fakeAccess{match: false}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: fake.PublicKey is refused to 'fake:invoke' by fakeAccessControl: not authorized")

	page.store["arc"] = &fakeAccess{match: true}
	task.ClientTask = InvokeTask{Key: []byte("y")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't invoke: oops")

	// 3. Consume a delete task.
	task.ClientTask = DeleteTask{Key: []byte("a")}

	err = task.Consume(fakeContext{}, page)
	require.NoError(t, err)

	task.ClientTask = DeleteTask{Key: []byte("c")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: invalid message type '<nil>' != '*contract.Instance'")

	// 4. Consume an invalid task.
	page.errWrite = xerrors.New("oops")
	task.ClientTask = DeleteTask{Key: []byte("a")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't write instance to page: oops")

	task.ClientTask = nil
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "invalid task type '<nil>'")
}

func TestTaskFactory_Register(t *testing.T) {
	factory := NewTaskFactory()

	factory.Register("a", fakeContract{})
	factory.Register("b", fakeContract{})
	require.Len(t, factory.contracts, 2)

	factory.Register("a", fakeContract{})
	require.Len(t, factory.contracts, 2)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstance(t *testing.T) *Instance {
	return &Instance{
		ContractID:    "fake",
		AccessControl: []byte("arc"),
		Deleted:       false,
		Value:         fake.Message{},
	}
}

type fakeContract struct {
	Contract
	err error
}

func (c fakeContract) Spawn(ctx SpawnContext) (serde.Message, []byte, error) {
	ctx.Read([]byte{0xab})
	return fake.Message{}, []byte("arc"), c.err
}

func (c fakeContract) Invoke(ctx InvokeContext) (serde.Message, error) {
	ctx.Read([]byte{0xab})
	return fake.Message{}, c.err
}

type fakePage struct {
	inventory.WritablePage
	store    map[string]serde.Message
	errRead  error
	errWrite error
}

func (page fakePage) Read(key []byte) (serde.Message, error) {
	return page.store[string(key)], page.errRead
}

func (page fakePage) Write(key []byte, value serde.Message) error {
	page.store[string(key)] = value
	return page.errWrite
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
	serde.UnimplementedMessage

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
