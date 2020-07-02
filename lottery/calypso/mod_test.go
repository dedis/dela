package calypso

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/util/random"
	"golang.org/x/xerrors"
)

func TestMain(t *testing.T) {
	calypso := NewCalypso(fakeDKG{})

	ca := fake.NewAuthority(0, ed25519.NewSigner)

	err := calypso.Listen()
	require.NoError(t, err)

	pubKey, err := calypso.Setup(ca, 0)
	require.NoError(t, err)

	message := []byte("Hello, world")
	K, C, err := encrypt(pubKey, message)
	require.NoError(t, err)

	// DARC stuff
	ownerID := fakeIdentity{buffer: []byte("owner")}
	foreignID := fakeIdentity{buffer: []byte("foreigner")}
	d := darc.NewAccess()
	d, err = d.Evolve(ArcRuleUpdate, ownerID)
	require.NoError(t, err)
	d, err = d.Evolve(ArcRuleRead, ownerID)
	require.NoError(t, err)

	encrypted := fakeEncryptedMessage{K: K, C: C}
	id, err := calypso.Write(encrypted, d)
	require.NoError(t, err)

	// Trying to read with the foreignID, which isn't allowed yet
	idents := []arc.Identity{foreignID}
	_, err = calypso.Read(id, idents...)
	require.EqualError(t, err, "darc verification failed: couldn't match 'calypso_read': couldn't match identity 'foreigner'")

	// update the acess to allow the foreignID to read
	d, err = d.Evolve(ArcRuleRead, ownerID, foreignID)
	require.NoError(t, err)
	err = calypso.UpdateAccess(id, ownerID, d)
	require.NoError(t, err)

	// now the foreignID whould be able to read
	decrypted, err := calypso.Read(id, idents...)
	require.NoError(t, err)

	require.Equal(t, message, decrypted)
}

// -----------------------------------------------------------------------------
// Utility functions

func encrypt(pubKey kyber.Point, message []byte) (K, C kyber.Point, err error) {
	// Embed the message (or as much of it as will fit) into a curve point.
	M := suite.Point().Embed(message, random.New())
	max := suite.Point().EmbedLen()
	if len(message) > max {
		return nil, nil, xerrors.Errorf("message too long")
	}

	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := suite.Scalar().Pick(random.New()) // ephemeral private key
	K = suite.Point().Mul(k, nil)          // ephemeral DH public key
	S := suite.Point().Mul(k, pubKey)      // ephemeral DH shared secret
	C = S.Add(S, M)                        // message blinded with secret

	return K, C, nil
}

//
// DKG
//

type fakeDKG struct {
}

func (f fakeDKG) Listen() (dkg.Actor, error) {
	return &fakeActor{}, nil
}

type fakeActor struct {
	privKey kyber.Scalar
	pubKey  kyber.Point
}

func (f *fakeActor) Setup(ca crypto.CollectiveAuthority, threshold int) (kyber.Point, error) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)
	f.privKey = privKey
	f.pubKey = pubKey

	return pubKey, nil
}

func (f *fakeActor) GetPublicKey() (kyber.Point, error) {
	return f.pubKey, nil
}

func (f fakeActor) Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error) {
	k, c, err := encrypt(f.pubKey, message)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to encrypt: %v", err)
	}

	return k, c, nil, nil
}

func (f fakeActor) Decrypt(K, C kyber.Point) ([]byte, error) {
	S := suite.Point().Mul(f.privKey, K) // regenerate shared secret
	M := suite.Point().Sub(C, S)         // use to un-blind the message
	message, err := M.Data()             // extract the embedded data
	if err != nil {
		return nil, xerrors.Errorf("failed to extract embedded data: %v", err)
	}

	return message, nil
}

func (f fakeActor) Reshare() error {
	return nil
}

//
// arc.Identity
//

type fakeIdentity struct {
	arc.Identity
	buffer []byte
	err    error
}

func (i fakeIdentity) MarshalText() ([]byte, error) {
	return i.buffer, i.err
}

func (i fakeIdentity) String() string {
	return string(i.buffer)
}

//
// Encrypted message
//

type fakeEncryptedMessage struct {
	K kyber.Point
	C kyber.Point
}

func (f fakeEncryptedMessage) GetK() kyber.Point {
	return f.K
}

func (f fakeEncryptedMessage) GetC() kyber.Point {
	return f.C
}
