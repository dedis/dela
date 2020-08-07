package pedersen

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/kyber/v3"
)

func TestPedersen_Listen(t *testing.T) {
	pedersen, _ := NewPedersen(fake.NewBadMino())
	_, err := pedersen.Listen()
	require.EqualError(t, err, "failed to create RPC: fake error")

	pedersen, _ = NewPedersen(fake.Mino{})
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
	require.EqualError(t, err, "failed to stream: fake error")

	rpc := fake.NewStreamRPC(fake.NewReceiver(), fake.NewBadSender())
	actor.rpc = rpc

	_, err = actor.Setup(fakeAuthority, 0)
	require.EqualError(t, err, "expected ed25519.PublicKey, got 'fake.PublicKey'")

	rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	actor.rpc = rpc

	fakeAuthority = fake.NewAuthority(2, ed25519.NewSigner)

	_, err = actor.Setup(fakeAuthority, 1)
	require.EqualError(t, err, "got an error from '%!s(<nil>)' while receiving: fake error")

	rpc = fake.NewStreamRPC(fake.NewReceiver(), fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Setup(fakeAuthority, 1)
	require.EqualError(t, err, "expected to receive a Done message, but go the following: <nil>")

	rpc = fake.NewStreamRPC(fake.NewReceiver(
		types.NewStartDone(suite.Point()),
		types.NewStartDone(suite.Point().Pick(suite.RandomStream())),
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

	actor.startRes = &state{participants: []mino.Address{fake.NewAddress(0)}, distrKey: suite.Point()}
	_, err = actor.GetPublicKey()
	require.NoError(t, err)
}

func TestPedersen_Decrypt(t *testing.T) {
	actor := Actor{
		rpc:      fake.NewBadRPC(),
		startRes: &state{participants: []mino.Address{fake.NewAddress(0)}, distrKey: suite.Point()},
	}

	_, err := actor.Decrypt(suite.Point(), suite.Point())
	require.NoError(t, err)

	rpc := fake.NewStreamRPC(fake.NewBadReceiver(), fake.NewBadSender())
	actor.rpc = rpc

	_, err = actor.Decrypt(suite.Point(), suite.Point())
	require.EqualError(t, err, "failed to receive from '%!s(<nil>)': fake error")

	rpc = fake.NewStreamRPC(fake.NewReceiver(), fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Decrypt(suite.Point(), suite.Point())
	require.EqualError(t, err, "got unexpected reply, expected a decrypt reply but got: <nil>")

	rpc = fake.NewStreamRPC(fake.NewReceiver(types.DecryptReply{I: -1, V: suite.Point()}), fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Decrypt(suite.Point(), suite.Point())
	require.EqualError(t, err, "failed to recover commit: share: not enough "+
		"good public shares to reconstruct secret commitment")

	rpc = fake.NewStreamRPC(fake.NewReceiver(types.DecryptReply{I: 1, V: suite.Point()}), fake.Sender{})
	actor.rpc = rpc

	_, err = actor.Decrypt(suite.Point(), suite.Point())
	require.NoError(t, err)
}

func TestPedersen_Reshare(t *testing.T) {
	actor := Actor{}
	actor.Reshare()
}

func TestPedersen_Scenario(t *testing.T) {
	n := 5

	addrFactory := minogrpc.AddressFactory{}

	treeFactory := routing.NewTreeRoutingFactory(3, addrFactory)

	minos := make([]mino.Mino, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {

		port := uint16(2000 + i)
		minogrpc, err := minogrpc.NewMinogrpc("127.0.0.1", port, treeFactory)
		require.NoError(t, err)

		defer minogrpc.GracefulClose()

		minos[i] = minogrpc
		addrs[i] = minogrpc.GetAddress()
	}

	pubkeys := make([]kyber.Point, len(minos))

	for i, mino := range minos {
		for _, m := range minos {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(), m.(*minogrpc.Minogrpc).GetCertificate())
		}

		dkg, pubkey := NewPedersen(mino.(*minogrpc.Minogrpc))

		dkgs[i] = dkg
		pubkeys[i] = pubkey
	}

	fakeAuthority := NewAuthority(addrs, pubkeys)

	message := []byte("Hello world")
	actors := make([]dkg.Actor, n)
	for i := 0; i < n; i++ {
		actor, err := dkgs[i].Listen()
		require.NoError(t, err)

		actors[i] = actor
	}

	// trying to call a decrpyt/encrypt before a setup
	_, _, _, err := actors[0].Encrypt(message)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")
	_, err = actors[0].Decrypt(nil, nil)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")

	_, err = actors[0].Setup(fakeAuthority, n)
	require.NoError(t, err)

	_, err = actors[0].Setup(fakeAuthority, n)
	require.EqualError(t, err, "startRes is already done, only one setup call is allowed")

	// every node should be able to encrypt/decrypt
	for i := 0; i < n; i++ {
		K, C, remainder, err := actors[i].Encrypt(message)
		require.NoError(t, err)
		require.Len(t, remainder, 0)

		decrypted, err := actors[i].Decrypt(K, C)
		require.NoError(t, err)

		require.Equal(t, message, decrypted)
	}
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
