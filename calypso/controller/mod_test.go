package controller

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/calypso"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

func TestMinimal_Build(t *testing.T) {
	minimal := NewMinimal()
	minimal.SetCommands(fakeBuilder{})
}

func TestMinimal_Inject(t *testing.T) {
	minimal := minimal{}
	inj := newInjector(fakeDKG{})
	err := minimal.Inject(fakeFlags{}, inj)
	require.NoError(t, err)

	require.Len(t, inj.(*fakeInjector).history, 1)
	require.IsType(t, &calypso.Caly{}, inj.(*fakeInjector).history[0])

	err = minimal.Inject(fakeFlags{}, newBadInjector())
	require.EqualError(t, err, "failed to resolve dkg: oops")

	err = minimal.Inject(fakeFlags{}, newInjector(fakeDKG{isBad: true}))
	require.EqualError(t, err, "failed to listen dkg: oops")
}

func TestSetupAction_GenerateRequest(t *testing.T) {
	formatter = fakeFormatter{}
	setup := setupAction{}

	flags := fakeFlags{
		Strings: map[string]string{
			"pubkeys": "aef123",
			"addrs":   "fake",
		},
		Ints: map[string]int{
			"threshold": 2,
		},
	}
	buf, err := setup.GenerateRequest(flags)
	require.NoError(t, err)
	require.Equal(t, "{\"Threshold\":2,\"Pubkeys\":[\"aef123\"],\"Addrs\":[\"fake\"]}", string(buf))

	formatter = fakeFormatter{badMarshal: true}
	_, err = setup.GenerateRequest(flags)
	require.EqualError(t, err, "failed to marshal the request: oops")

	flags.Strings["pubkeys"] = "aef123,deadbeef"
	formatter = fakeFormatter{}
	_, err = setup.GenerateRequest(flags)
	require.EqualError(t, err, "there should be the same number of pubkkeys and addrs, but got 2 pubkeys and 1 addrs: {2 [aef123 deadbeef] [fake]}")

	flags.Ints["threshold"] = -1
	formatter = fakeFormatter{}
	_, err = setup.GenerateRequest(flags)
	require.EqualError(t, err, "threshold wrong or not provided: -1")

	flags.Strings["addrs"] = ""
	formatter = fakeFormatter{}
	_, err = setup.GenerateRequest(flags)
	require.EqualError(t, err, "addrs not found")

	flags.Strings["pubkeys"] = ""
	formatter = fakeFormatter{}
	_, err = setup.GenerateRequest(flags)
	require.EqualError(t, err, "pubkeys not found")
}

func TestSetupAction_Execute(t *testing.T) {
	formatter = fakeFormatter{
		executeRequest: executeRequest{
			Addrs:   []string{"127.0.0.1"},
			Pubkeys: []string{"0000000000000000000000000000000000000000000000000000000000000000"},
		},
	}
	setup := setupAction{}
	ctx := node.Context{
		Injector: &fakeInjector{
			mino: fake.Mino{},
			privateStorage: fakePrivateStorage{
				setupPoint: dkg.Suite.Point(),
			},
		},
	}

	err := setup.Execute(ctx)
	require.NoError(t, err)

	ctx = node.Context{
		Injector: &fakeInjector{
			mino: fake.Mino{},
			privateStorage: fakePrivateStorage{
				setupPoint: badPoint{},
			},
		},
	}

	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to mashal pubkey: oops")

	ctx = node.Context{
		Injector: &fakeInjector{
			mino: fake.Mino{},
			privateStorage: fakePrivateStorage{
				badSetup: true,
			},
		},
	}

	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to setup calypso: oops")

	formatter = fakeFormatter{
		executeRequest: executeRequest{
			Addrs:   []string{"127.0.0.1"},
			Pubkeys: []string{"aef123"},
		},
	}

	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to unmarhsal point: invalid Ed25519 curve point")

	formatter = fakeFormatter{
		executeRequest: executeRequest{
			Addrs:   []string{"127.0.0.1"},
			Pubkeys: []string{"xyz"},
		},
	}

	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to decode hex key: encoding/hex: invalid byte: U+0078 'x'")

	formatter = fakeFormatter{badDecode: true}
	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to get the request: oops")

	ctx = node.Context{
		Injector: &fakeInjector{
			mino: fake.Mino{},
		},
	}

	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to resolve calypso: oops")

	ctx = node.Context{
		Injector: &fakeInjector{},
	}

	err = setup.Execute(ctx)
	require.EqualError(t, err, "failed to resolve mino: oops")
}

func TestJsonFormatter_Marshal(t *testing.T) {
	formatter := jsonFormatter{}
	test := struct{ Dummy string }{"hello"}
	res, err := formatter.Marshal(&test)
	require.NoError(t, err)
	require.Equal(t, string(res), "{\"Dummy\":\"hello\"}")
}

func TestJsonFormatter_Decode(t *testing.T) {
	formatter := jsonFormatter{}
	test := struct{ Dummy string }{}
	reader := strings.NewReader("{\"Dummy\":\"hello\"}")
	err := formatter.Decode(&test, reader)
	require.NoError(t, err)
	require.Equal(t, "hello", test.Dummy)
}

// -----------------------------------------------------------------------------
// Utility functions

// fakeBuilder is a fake builders
//
// - implements node.Builder
type fakeBuilder struct {
}

// SetCommand implements node.Builder
func (f fakeBuilder) SetCommand(name string) cli.CommandBuilder {
	return fakeCommandBuilder{}
}

// SetStartFlags implements node.Builder
func (f fakeBuilder) SetStartFlags(_ ...cli.Flag) {
}

// MakeNodeAction implements node.Builder
func (f fakeBuilder) MakeAction(_ node.ActionTemplate) cli.Action {
	return nil
}

// fakeCommandBuilder is a fake command builder
//
// - implements cli.CommandBuilder
type fakeCommandBuilder struct {
}

// SetDescription cli.CommandBuilder
func (b fakeCommandBuilder) SetDescription(value string) {
}

// SetFlags cli.CommandBuilder
func (b fakeCommandBuilder) SetFlags(_ ...cli.Flag) {
}

// SetAction cli.CommandBuilder
func (b fakeCommandBuilder) SetAction(_ cli.Action) {
}

// SetSubCommand cli.CommandBuilder
func (b fakeCommandBuilder) SetSubCommand(name string) cli.CommandBuilder {
	return fakeCommandBuilder{}
}

// newInjector is used to test the inject function, which requires an interface
func newInjector(dkg dkg.DKG) node.Injector {
	return &fakeInjector{
		dkg: dkg,
	}
}

func newBadInjector() node.Injector {
	return &fakeInjector{
		isBad: true,
	}
}

// fakeInjector is a fake injector
//
// - implements node.Injector
type fakeInjector struct {
	isBad          bool
	dkg            dkg.DKG
	mino           mino.Mino
	privateStorage calypso.PrivateStorage
	history        []interface{}
}

// Resolve implements node.Injector
func (i fakeInjector) Resolve(el interface{}) error {
	if i.isBad {
		return xerrors.New("oops")
	}

	switch msg := el.(type) {
	case *dkg.DKG:
		*msg = i.dkg
	case *mino.Mino:
		if i.mino == nil {
			return xerrors.New("oops")
		}
		*msg = i.mino
	case *calypso.PrivateStorage:
		if i.privateStorage == nil {
			return xerrors.New("oops")
		}
		*msg = i.privateStorage
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

// fakeFlags is a fake flags
//
// - implements cli.Flags
type fakeFlags struct {
	Strings map[string]string
	Ints    map[string]int
}

// String implements cli.Flags
func (f fakeFlags) String(name string) string {
	return f.Strings[name]
}

// Duration implements cli.Flags
func (f fakeFlags) Duration(name string) time.Duration {
	panic("not implemented") // TODO: Implement
}

// Path implements cli.Flags
func (f fakeFlags) Path(name string) string {
	panic("not implemented") // TODO: Implement
}

// Int implements cli.Flags
func (f fakeFlags) Int(name string) int {
	return f.Ints[name]
}

// fakeDKG is a fake DKG
//
// - implements dkg.DKG
type fakeDKG struct {
	isBad bool
}

// Listen implements dkg.DKG
func (dkg fakeDKG) Listen() (dkg.Actor, error) {
	if dkg.isBad {
		return nil, xerrors.Errorf("oops")
	}

	return nil, nil
}

// fakeFormatter is a formatter using json
//
// - implements formatterI
type fakeFormatter struct {
	badMarshal     bool
	badDecode      bool
	executeRequest executeRequest
}

func (f fakeFormatter) Marshal(i interface{}) ([]byte, error) {
	if f.badMarshal {
		return nil, xerrors.New("oops")
	}

	return json.Marshal(i)
}

// Decode decodes i using json
//
// - implements formatterI
func (f fakeFormatter) Decode(i interface{}, reader io.Reader) error {
	if f.badDecode {
		return xerrors.New("oops")
	}

	el, ok := i.(*executeRequest)
	if ok {
		*el = f.executeRequest
		return nil
	}

	dec := json.NewDecoder(reader)
	return dec.Decode(i)
}

// fakePrivateStorage is a fake private storage
//
// implements calypso.PrivateStorage
type fakePrivateStorage struct {
	setupPoint kyber.Point
	badSetup   bool
}

// Setup implements calypso.PrivateStorage
func (ps fakePrivateStorage) Setup(ca crypto.CollectiveAuthority, threshold int) (pubKey kyber.Point, err error) {
	if ps.badSetup {
		return nil, xerrors.Errorf("oops")
	}

	return ps.setupPoint, nil
}

// GetPublicKey implements calypso.PrivateStorage
func (ps fakePrivateStorage) GetPublicKey() (kyber.Point, error) {
	panic("not implemented") // TODO: Implement
}

// Write implements calypso.PrivateStorage
func (ps fakePrivateStorage) Write(message calypso.EncryptedMessage, ac arc.AccessControl) (ID []byte, err error) {
	panic("not implemented") // TODO: Implement
}

// Read implements calypso.PrivateStorage
func (ps fakePrivateStorage) Read(ID []byte, idents ...arc.Identity) (msg []byte, err error) {
	panic("not implemented") // TODO: Implement
}

// UpdateAccess implements calypso.PrivateStorage
func (ps fakePrivateStorage) UpdateAccess(ID []byte, ident arc.Identity, ac arc.AccessControl) error {
	panic("not implemented") // TODO: Implement
}

// badPoint is a bad kyber point
//
// implements kyber.Point
type badPoint struct {
	kyber.Point
}

// MarshalBinary implements kyber.Point
func (badPoint) MarshalBinary() ([]byte, error) {
	return nil, xerrors.Errorf("oops")
}
