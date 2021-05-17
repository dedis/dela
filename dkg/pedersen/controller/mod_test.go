package controller

import (
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/dkg/pedersen"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
	"testing"
)

func TestController_SetCommands(t *testing.T) {
	c := NewController()

	call := &fake.Call{}
	c.SetCommands(fakeBuilder{call: call})

	require.Equal(t, 34, call.Len())
	require.Equal(t, "dkg", call.Get(0, 0))
	require.Equal(t, "interact with the DKG service", call.Get(1, 0))

	require.Equal(t, "init", call.Get(2, 0))
	require.Equal(t, "initialize the DKG protocol", call.Get(3, 0))
	require.IsType(t, &initAction{}, call.Get(4, 0))
	require.Nil(t, call.Get(5, 0))

	require.Equal(t, "setup", call.Get(6, 0))
	require.Equal(t, "creates the public distributed key and the private share on each node", call.Get(7, 0))
	require.Len(t, call.Get(8, 0), 1)
	require.IsType(t, &setupAction{}, call.Get(9, 0))
	require.Nil(t, call.Get(10, 0))

	require.Equal(t, "initHttpServer", call.Get(11, 0))
	require.Equal(t, "initialize the DKG service HTTP server", call.Get(12, 0))
	require.Len(t, call.Get(13, 0), 1)
	require.IsType(t, &initHttpServerAction{}, call.Get(14, 0))
	require.Nil(t, call.Get(15, 0))

	require.Equal(t, "export", call.Get(16, 0))
	require.Equal(t, "export the node address and public key", call.Get(17, 0))
	require.IsType(t, &exportInfoAction{}, call.Get(18, 0))
	require.Nil(t, call.Get(19, 0))

	require.Equal(t, "getPublicKey", call.Get(20, 0))
	require.Equal(t, "prints the distributed public Key", call.Get(21, 0))
	require.IsType(t, &getPublicKeyAction{}, call.Get(22, 0))
	require.Nil(t, call.Get(23, 0))

	require.Equal(t, "encrypt", call.Get(24, 0))
	require.Equal(t, "encrypt the given string and write the ciphertext pair in the corresponding file", call.Get(25, 0))
	require.Len(t, call.Get(26, 0), 3)
	require.IsType(t, &encryptAction{}, call.Get(27, 0))
	require.Nil(t, call.Get(28, 0))

	require.Equal(t, "decrypt", call.Get(29, 0))
	require.Equal(t, "decrypt the given ciphertext pair and print the corresponding plaintext", call.Get(30, 0))
	require.Len(t, call.Get(31, 0), 2)
	require.IsType(t, &decryptAction{}, call.Get(32, 0))
	require.Nil(t, call.Get(33, 0))
}

func TestMinimal_OnStart(t *testing.T) {
	c := NewController()
	inj := newInjector(nil)

	err := c.OnStart(nil, inj)
	require.EqualError(t, err, fake.Err("failed to resolve mino"))

	inj = newInjector(fake.Mino{})
	err = c.OnStart(nil, inj)
	require.NoError(t, err)
	require.Len(t, inj.(*fakeInjector).history, 2)
	require.IsType(t, &pedersen.Pedersen{}, inj.(*fakeInjector).history[0])
	pubkey := suite.Point()
	require.IsType(t, pubkey, inj.(*fakeInjector).history[1])
}

func TestMinimal_OnStop(t *testing.T) {
	c := NewController()

	err := c.OnStop(node.NewInjector())
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeCommandBuilder struct {
	call *fake.Call
}

func (b fakeCommandBuilder) SetSubCommand(name string) cli.CommandBuilder {
	b.call.Add(name)
	return b
}

func (b fakeCommandBuilder) SetDescription(value string) {
	b.call.Add(value)
}

func (b fakeCommandBuilder) SetFlags(flags ...cli.Flag) {
	b.call.Add(flags)
}

func (b fakeCommandBuilder) SetAction(a cli.Action) {
	b.call.Add(a)
}

type fakeBuilder struct {
	call *fake.Call
}

func (b fakeBuilder) SetCommand(name string) cli.CommandBuilder {
	b.call.Add(name)
	return fakeCommandBuilder(b)
}

func (b fakeBuilder) SetStartFlags(flags ...cli.Flag) {
	b.call.Add(flags)
}

func (b fakeBuilder) MakeAction(tmpl node.ActionTemplate) cli.Action {
	b.call.Add(tmpl)
	return nil
}

func newInjector(mino mino.Mino) node.Injector {
	return &fakeInjector{
		mino: mino,
	}
}

// fakeInjector is a fake injector
//
// - implements node.Injector
type fakeInjector struct {
	isBad   bool
	mino    mino.Mino
	history []interface{}
}

// Resolve implements node.Injector
func (i fakeInjector) Resolve(el interface{}) error {
	if i.isBad {
		return fake.GetError()
	}

	switch msg := el.(type) {
	case *mino.Mino:
		if i.mino == nil {
			return fake.GetError()
		}
		*msg = i.mino
	default:
		return xerrors.Errorf("unkown message '%T", msg)
	}

	return nil
}

// Inject implements node.Injector
func (i *fakeInjector) Inject(v interface{}) {
	if i.history == nil {
		i.history = make([]interface{}, 0)
	}
	i.history = append(i.history, v)
}
