package pedersen

import (
	"fmt"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/key"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/kyber/v3"
)

func TestPedersen_Listen(t *testing.T) {
	pedersen, _ := NewPedersen(fake.Mino{})

	actor, err := pedersen.Listen()
	require.NoError(t, err)

	require.NotNil(t, actor)
}

func TestPedersen_Setup(t *testing.T) {
	actor := Actor{
		rpc:      fake.NewBadRPC(),
		startRes: &state{},
	}

	fakeAuthority := fake.NewAuthority(1, fake.NewSigner)

	_, err := actor.Setup(fakeAuthority, 0)
	require.EqualError(t, err, fake.Err("failed to stream"))

	rpc := fake.NewStreamRPC(fake.NewReceiver(), fake.NewBadSender())
	actor.rpc = rpc

	_, err = actor.Setup(fakeAuthority, 0)
	require.EqualError(t, err, "expected ed25519.PublicKey, got 'fake.PublicKey'")

	fakeAuthority = fake.NewAuthority(2, ed25519.NewSigner)

	_, err = actor.Setup(fakeAuthority, 1)
	require.EqualError(t, err, fake.Err("failed to send start"))

	rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Setup(fakeAuthority, 1)
	require.EqualError(t, err, fake.Err("got an error from '%!s(<nil>)' while receiving"))

	recv := fake.NewReceiver(fake.NewRecvMsg(fake.NewAddress(0), nil))

	actor.rpc = fake.NewStreamRPC(recv, fake.Sender{})

	_, err = actor.Setup(fakeAuthority, 1)
	require.EqualError(t, err, "expected to receive a Done message, but go the following: <nil>")

	rpc = fake.NewStreamRPC(fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.NewStartDone(suite.Point())),
		fake.NewRecvMsg(fake.NewAddress(0), types.NewStartDone(suite.Point().Pick(suite.RandomStream()))),
	), fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Setup(fakeAuthority, 1)
	require.Error(t, err)
	require.Regexp(t, "^the public keys does not match:", err)
}

func TestPedersen_GetPublicKey(t *testing.T) {
	actor := Actor{
		startRes: &state{},
	}

	_, err := actor.GetPublicKey()
	require.EqualError(t, err, "DKG has not been initialized")

	actor.startRes = &state{dkgState: certified}
	_, err = actor.GetPublicKey()
	require.NoError(t, err)
}

func TestPedersen_Decrypt(t *testing.T) {
	actor := Actor{
		rpc: fake.NewBadRPC(),
		startRes: &state{dkgState: certified,
			participants: []mino.Address{fake.NewAddress(0)}, distrKey: suite.Point()},
	}

	K := suite.Point()
	Cs := make([]kyber.Point, 1)

	_, err := actor.Decrypt(K, Cs)
	require.EqualError(t, err, fake.Err("failed to create stream"))

	rpc := fake.NewStreamRPC(fake.NewBadReceiver(), fake.NewBadSender())
	actor.rpc = rpc

	_, err = actor.Decrypt(K, Cs)
	require.EqualError(t, err, fake.Err("failed to send decrypt request"))

	recv := fake.NewReceiver(fake.NewRecvMsg(fake.NewAddress(0), nil))

	rpc = fake.NewStreamRPC(recv, fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Decrypt(K, Cs)
	require.EqualError(t, err, "got unexpected reply, expected types.DecryptReply but got: <nil>")

	recv = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.DecryptReply{I: -1, V: suite.Point()}),
	)

	rpc = fake.NewStreamRPC(recv, fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Decrypt(K, Cs)
	require.EqualError(t, err, "failed to recover commit: share: not enough "+
		"good public shares to reconstruct secret commitment")

	recv = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.DecryptReply{I: 1, V: suite.Point()}),
	)

	rpc = fake.NewStreamRPC(recv, fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Decrypt(K, Cs)
	require.NoError(t, err)
}

func Test_Decrypt_StreamStop(t *testing.T) {
	a := Actor{
		rpc: fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{}),
		startRes: &state{
			dkgState:     certified,
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}

	Cs := make([]kyber.Point, 1)

	_, err := a.Decrypt(nil, Cs)
	require.EqualError(t, err, fake.Err("stream stopped unexpectedly"))
}

func TestPedersen_Scenario(t *testing.T) {
	// Use with MINO_TRAFFIC=log
	// traffic.LogItems = false
	// traffic.LogEvent = false
	// defer func() {
	// 	traffic.SaveItems("graph.dot", true, false)
	// 	traffic.SaveEvents("events.dot")
	// }()

	oldLog := dela.Logger
	defer func() {
		dela.Logger = oldLog
	}()

	dela.Logger = dela.Logger.Level(zerolog.WarnLevel)

	n := 32

	minos := make([]mino.Mino, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {
		addr := minogrpc.ParseAddress("127.0.0.1", 0)

		m, err := minogrpc.NewMinogrpc(addr, nil, tree.NewRouter(minogrpc.NewAddressFactory()))
		require.NoError(t, err)

		defer m.GracefulStop()

		minos[i] = m
		addrs[i] = m.GetAddress()
	}

	pubkeys := make([]kyber.Point, len(minos))

	for i, mi := range minos {
		for _, m := range minos {
			mi.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(), m.(*minogrpc.Minogrpc).GetCertificateChain())
		}

		d, pubkey := NewPedersen(mi.(*minogrpc.Minogrpc))

		dkgs[i] = d
		pubkeys[i] = pubkey
	}

	actors := make([]dkg.Actor, n)
	for i := 0; i < n; i++ {
		actor, err := dkgs[i].Listen()
		require.NoError(t, err)

		actors[i] = actor
	}

	// trying to call a decrypt/encrypt before a setup
	message := []byte("Hello world")

	_, _, err := actors[0].Encrypt(message)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")
	_, err = actors[0].Decrypt(nil, nil)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")

	fakeAuthority := NewAuthority(addrs, pubkeys)

	_, err = actors[0].Setup(fakeAuthority, n)
	require.NoError(t, err)

	_, err = actors[0].Setup(fakeAuthority, n)
	require.EqualError(t, err, "startRes is already done, only one setup call is allowed")

	// every node should be able to encrypt/decrypt
	for i := 0; i < n; i++ {
		K, C, err := actors[i].Encrypt(message)
		require.NoError(t, err)
		decrypted, err := actors[i].Decrypt(K, C)
		require.NoError(t, err)
		require.Equal(t, message, decrypted)
	}
}

func TestPedersen_ReencryptScenario(t *testing.T) {
	// Use with MINO_TRAFFIC=log
	// traffic.LogItems = false
	// traffic.LogEvent = false
	// defer func() {
	// 	traffic.SaveItems("graph.dot", true, false)
	// 	traffic.SaveEvents("events.dot")
	// }()

	oldLog := dela.Logger
	defer func() {
		dela.Logger = oldLog
	}()

	dela.Logger = dela.Logger.Level(zerolog.WarnLevel)

	nbNodes := 7

	minos := make([]mino.Mino, nbNodes)
	dkgs := make([]dkg.DKG, nbNodes)
	addrs := make([]mino.Address, nbNodes)

	manager := minoch.NewManager()

	for i := 0; i < nbNodes; i++ {
		minos[i] = minoch.MustCreate(manager, fmt.Sprint("node", i))
		addrs[i] = minos[i].GetAddress()
	}

	pubkeys := make([]kyber.Point, len(minos))

	for i, mi := range minos {
		d, pubkey := NewPedersen(mi.(*minoch.Minoch))
		dkgs[i] = d
		pubkeys[i] = pubkey
	}

	actors := make([]dkg.Actor, nbNodes)
	for i := 0; i < nbNodes; i++ {
		actor, err := dkgs[i].Listen()
		require.NoError(t, err)

		actors[i] = actor
	}

	fakeAuthority := NewAuthority(addrs, pubkeys)
	_, err := actors[0].Setup(fakeAuthority, nbNodes)
	require.NoError(t, err)

	// every node should be able to encrypt/reencrypt/decrypt
	kp := key.NewKeyPair(suites.MustFind("Ed25519"))

	for i := 0; i < nbNodes; i++ {
		message := []byte(fmt.Sprint("Hello world, I'm", i))
		U, Cs := actors[i].EncryptSecret(message)

		XhatEnc, err := actors[i].ReencryptSecret(U, kp.Public)
		require.NoError(t, err)
		require.NotNil(t, XhatEnc)

		decrypted, err := actors[i].DecryptSecret(Cs, XhatEnc, kp.Private)
		require.NoError(t, err)
		require.Equal(t, message, decrypted)
	}
}

func Test_Worker_BadProof(t *testing.T) {
	ct := types.Ciphertext{
		K:    suite.Point(),
		C:    suite.Point(),
		UBar: suite.Point(),
		E:    suite.Scalar(),
		F:    suite.Scalar(),
		GBar: suite.Point(),
	}

	sap := types.ShareAndProof{
		V:  suite.Point(),
		I:  0,
		Ui: suite.Point(),
		Ei: suite.Scalar(),
		Fi: suite.Scalar(),
		Hi: suite.Point(),
	}

	w := worker{
		numParticipants:  0,
		decryptedMessage: [][]byte{},
		ciphertexts: []types.Ciphertext{
			ct,
		},
		responses: []types.VerifiableDecryptReply{types.NewVerifiableDecryptReply([]types.ShareAndProof{sap})},
	}

	err := w.work(0)
	require.Regexp(t, "^failed to check the decryption proof: hash is not valid", err.Error())
}

func Test_Worker_BadRecover(t *testing.T) {
	w := worker{
		numParticipants:  2,
		decryptedMessage: [][]byte{},
		ciphertexts:      []types.Ciphertext{},
		responses:        []types.VerifiableDecryptReply{},
	}

	err := w.work(0)
	require.Regexp(t, "^failed to recover the commit:", err.Error())
}

func Test_Reshare_NotDone(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: initial},
	}

	err := a.Reshare(nil, 0)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")
}

func Test_Reshare_WrongPK(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
	}

	co := fake.NewAuthority(1, fake.NewSigner)

	err := a.Reshare(co, 0)
	require.EqualError(t, err, "expected ed25519.PublicKey, got 'fake.PublicKey'")
}

func Test_Reshare_BadRPC(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
		rpc:      fake.NewBadRPC(),
	}

	co := NewAuthority(nil, nil)

	err := a.Reshare(co, 0)
	require.EqualError(t, err, fake.Err("failed to create stream"))
}

func Test_Reshare_BadSender(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
		rpc:      fake.NewStreamRPC(nil, fake.NewBadSender()),
	}

	co := NewAuthority(nil, nil)

	err := a.Reshare(co, 0)
	require.EqualError(t, err, fake.Err("failed to send resharing request"))
}

func Test_Reshare_BadReceiver(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
		rpc:      fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{}),
	}

	co := NewAuthority([]mino.Address{fake.NewAddress(0)}, []kyber.Point{suite.Point()})

	err := a.Reshare(co, 0)
	require.EqualError(t, err, fake.Err("stream stopped unexpectedly"))
}

// -----------------------------------------------------------------------------
// Utility functions

//
// Collective authority
//

// CollectiveAuthority is a fake implementation of the cosi.CollectiveAuthority
// interface.
type CollectiveAuthority struct {
	crypto.CollectiveAuthority
	addrs   []mino.Address
	pubkeys []kyber.Point
	signers []crypto.Signer
}

// NewAuthority returns a new collective authority of n members with new signers
// generated by g.
func NewAuthority(addrs []mino.Address, pubkeys []kyber.Point) CollectiveAuthority {
	signers := make([]crypto.Signer, len(pubkeys))
	for i, pubkey := range pubkeys {
		signers[i] = newFakeSigner(pubkey)
	}

	return CollectiveAuthority{
		pubkeys: pubkeys,
		addrs:   addrs,
		signers: signers,
	}
}

// GetPublicKey implements cosi.CollectiveAuthority.
func (ca CollectiveAuthority) GetPublicKey(addr mino.Address) (crypto.PublicKey, int) {

	for i, address := range ca.addrs {
		if address.Equal(addr) {
			return ed25519.NewPublicKeyFromPoint(ca.pubkeys[i]), i
		}
	}
	return nil, -1
}

// Len implements mino.Players.
func (ca CollectiveAuthority) Len() int {
	return len(ca.pubkeys)
}

// AddressIterator implements mino.Players.
func (ca CollectiveAuthority) AddressIterator() mino.AddressIterator {
	return fake.NewAddressIterator(ca.addrs)
}

func (ca CollectiveAuthority) PublicKeyIterator() crypto.PublicKeyIterator {
	return fake.NewPublicKeyIterator(ca.signers)
}

func newFakeSigner(pubkey kyber.Point) fakeSigner {
	return fakeSigner{
		pubkey: pubkey,
	}
}

// fakeSigner is a fake signer
//
// - implements crypto.Signer
type fakeSigner struct {
	crypto.Signer
	pubkey kyber.Point
}

// GetPublicKey implements crypto.Signer
func (s fakeSigner) GetPublicKey() crypto.PublicKey {
	return ed25519.NewPublicKeyFromPoint(s.pubkey)
}
