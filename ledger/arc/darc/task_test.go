package darc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestClientTask_Fingerprint(t *testing.T) {
	task := ClientTask{
		key: []byte{0x01},
		access: Access{rules: map[string]Expression{
			"\x02": {matches: map[string]struct{}{"\x03": {}}},
		}},
	}

	buffer := new(bytes.Buffer)

	err := task.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x01\x02\x03", buffer.String())

	err = task.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write key: fake error")

	err = task.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err,
		"couldn't fingerprint access: couldn't write key: fake error")
}

func TestServerTask_Consume(t *testing.T) {
	access, err := NewAccess().Evolve(UpdateAccessRule, fakeIdentity{buffer: []byte("doggy")})
	require.NoError(t, err)

	task := ServerTask{
		ClientTask: ClientTask{key: []byte{0x01}, access: access},
	}

	call := &fake.Call{}
	err = task.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 1, call.Len())
	// Key is provided so it's an update.
	require.Equal(t, []byte{0x01}, call.Get(0, 0))

	// No key thus it's a creation.
	task.key = nil
	err = task.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())
	require.Equal(t, []byte{0x34}, call.Get(1, 0))

	err = task.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write access: oops")

	task.key = []byte{0x01}
	err = task.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read value: oops")

	err = task.Consume(fakeContext{}, badPage{})
	require.EqualError(t, err, "invalid message type 'fake.Message'")

	task.access.rules[UpdateAccessRule].matches["cat"] = struct{}{}
	err = task.Consume(fakeContext{identity: []byte("cat")}, fakePage{})
	require.EqualError(t, err,
		"no access: couldn't match 'darc_update': couldn't match identity 'cat'")
}

func TestRegister(t *testing.T) {
	factory := basic.NewTransactionFactory(fake.NewSigner())
	Register(factory, NewTaskFactory())
}

// -----------------------------------------------------------------------------
// Utility functions

var testAccess = Access{
	rules: map[string]Expression{
		UpdateAccessRule: {matches: map[string]struct{}{"doggy": {}}},
	},
}

type fakeContext struct {
	basic.Context
	identity []byte
}

func (ctx fakeContext) GetID() []byte {
	return []byte{0x34}
}

func (ctx fakeContext) GetIdentity() arc.Identity {
	if ctx.identity != nil {
		return fakeIdentity{buffer: ctx.identity}
	}
	return fakeIdentity{buffer: []byte("doggy")}
}

type fakePage struct {
	inventory.WritablePage
	call *fake.Call
	err  error
}

func (page fakePage) Read(key []byte) (serde.Message, error) {
	return testAccess, page.err
}

func (page fakePage) Write(key []byte, value serde.Message) error {
	page.call.Add(key, value)

	return page.err
}

type badPage struct {
	inventory.WritablePage
}

func (page badPage) Read([]byte) (serde.Message, error) {
	return fake.Message{}, nil
}
