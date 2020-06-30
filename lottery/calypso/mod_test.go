package calypso

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cothority/v3/darc"
	"go.dedis.ch/cothority/v3/darc/expression"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/util/random"
	"golang.org/x/xerrors"
)

func TestMain(t *testing.T) {
	calypso := NewCalypso(fakeDKG{})

	pubKey, err := calypso.Setup(&fakePlayers{}, []kyber.Point{}, 0)
	require.NoError(t, err)

	message := []byte("Hello, world")
	K, C, err := encrypt(pubKey, message)
	require.NoError(t, err)

	// DARC stuff
	owner := darc.NewSignerEd25519(nil, nil)
	rules := darc.InitRules([]darc.Identity{owner.Identity()}, []darc.Identity{})

	d := darc.NewDarc(rules, []byte("example darc"))
	expr := expression.InitAndExpr(owner.Identity().String())
	action := darc.Action("custom_action")
	d.Rules.AddRule(action, expr)
	r, err := darc.InitAndSignRequest(d.GetBaseID(), action, []byte("example request"), owner)
	require.NoError(t, err)

	encrypted := NewEncryptedMessage(K, C)
	id, err := calypso.Write(encrypted, d)
	require.NoError(t, err)

	decrypted, err := calypso.Read(id, *r)
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

func (f *fakeActor) Setup(players mino.Players, pubKeys []kyber.Point, threshold int) (kyber.Point, error) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)
	f.privKey = privKey
	f.pubKey = pubKey

	return pubKey, nil
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
// Players
//

type fakePlayers struct {
	players  []mino.Address
	iterator *fakeAddressIterator
}

// AddressIterator implements mino.Players
func (p *fakePlayers) AddressIterator() mino.AddressIterator {
	if p.iterator == nil {
		p.iterator = &fakeAddressIterator{players: p.players}
	}
	return p.iterator
}

// Len() implements mino.Players.Len()
func (p *fakePlayers) Len() int {
	return len(p.players)
}

// Take implements mino.Players
func (p *fakePlayers) Take(filters ...mino.FilterUpdater) mino.Players {
	f := mino.ApplyFilters(filters)
	players := make([]mino.Address, len(p.players))
	for i, k := range f.Indices {
		players[i] = p.players[k]
	}
	return &fakePlayers{
		players: players,
	}
}

//
// AddressIterator
//

type fakeAddressIterator struct {
	players []mino.Address
	cursor  int
}

// Seek implements mino.AddressIterator.
func (it *fakeAddressIterator) Seek(index int) {
	it.cursor = index
}

// HasNext implements mino.AddressIterator
func (it *fakeAddressIterator) HasNext() bool {
	return it.cursor < len(it.players)
}

// GetNext implements mino.AddressIterator.GetNext(). It is the responsibility
// of the caller to check there is still elements to get. Otherwise it may
// crash.
func (it *fakeAddressIterator) GetNext() mino.Address {
	p := it.players[it.cursor]
	it.cursor++
	return p
}
