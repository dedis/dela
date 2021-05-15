package neff

import (
	"context"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	orderingTypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	neffShuffleTypes "go.dedis.ch/dela/shuffle/neff/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/proof"
	shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/util/random"
	"strconv"
	"testing"
)

func TestNeffShuffle_Listen(t *testing.T) {

	NeffShuffle := NewNeffShuffle(fake.Mino{}, FakeOrdering{}, FakePool{}, FakeBlockStore{})

	actor, err := NeffShuffle.Listen()
	require.NoError(t, err)

	require.NotNil(t, actor)
}

func TestNeffShuffle_Shuffle(t *testing.T) {

	electionId := "dummyId"

	actor := Actor{
		rpc:  fake.NewBadRPC(),
		mino: fake.Mino{},
	}

	fakeAuthority := fake.NewAuthority(1, fake.NewSigner)

	err := actor.Shuffle(fakeAuthority, electionId)
	require.EqualError(t, err, fake.Err("failed to stream"))

	rpc := fake.NewStreamRPC(fake.NewReceiver(), fake.NewBadSender())
	actor.rpc = rpc

	err = actor.Shuffle(fakeAuthority, electionId)
	require.EqualError(t, err, fake.Err("failed to send first message"))

	rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	actor.rpc = rpc

	err = actor.Shuffle(fakeAuthority, electionId)
	require.EqualError(t, err, fake.Err("got an error from '%!s(<nil>)' while receiving"))

	recv := fake.NewReceiver(fake.NewRecvMsg(fake.NewAddress(0), nil))

	rpc = fake.NewStreamRPC(recv, fake.Sender{})
	actor.rpc = rpc

	err = actor.Shuffle(fakeAuthority, electionId)
	require.EqualError(t, err, "expected to receive an EndShuffle message, but go the following: <nil>")

	recv = fake.NewReceiver(fake.NewRecvMsg(fake.NewAddress(0), neffShuffleTypes.NewEndShuffle()))

	rpc = fake.NewStreamRPC(recv, fake.Sender{})
	actor.rpc = rpc

	err = actor.Shuffle(fakeAuthority, electionId)
	require.NoError(t, err)
}

func TestNeffShuffle_Verify(t *testing.T) {

	actor := Actor{
	}

	rand := suite.RandomStream()
	h := suite.Scalar().Pick(rand)
	H := suite.Point().Mul(h, nil)

	k := 3
	X := make([]kyber.Point, k)
	Y := make([]kyber.Point, k)

	for i := 0; i < k; i++ {
		// Embed the message into a curve point
		message := "Test" + strconv.Itoa(i)
		M := suite.Point().Embed([]byte(message), random.New())

		// ElGamal-encrypt the point to produce ciphertext (K,C).
		k := suite.Scalar().Pick(random.New()) // ephemeral private key
		K := suite.Point().Mul(k, nil)         // ephemeral DH public key
		S := suite.Point().Mul(k, H)           // ephemeral DH shared secret
		C := S.Add(S, M)                       // message blinded with secret
		X[i] = K
		Y[i] = C
	}

	Kbar, Cbar, prover := shuffleKyber.Shuffle(suite, nil, H, X, Y, rand)
	shuffleProof, _ := proof.HashProve(suite, protocolName, prover)

	err := actor.Verify(suite.String(), Y, Y, H, Kbar, Cbar, shuffleProof)
	require.EqualError(t, err, "invalid PairShuffleProof")

	err = actor.Verify(suite.String(), X, Y, H, Kbar, Cbar, shuffleProof)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

//
// Fake Ordering
//

type FakeOrdering struct {
}

func (f FakeOrdering) GetProof(key []byte) (ordering.Proof, error) {
	return nil, nil
}

func (f FakeOrdering) GetStore() store.Readable {
	return nil
}

func (f FakeOrdering) Watch(ctx context.Context) <-chan ordering.Event {
	return nil
}

func (f FakeOrdering) Close() error {
	return nil
}

//
// Fake Pool
//

type FakePool struct {
}

func (f FakePool) SetPlayers(players mino.Players) error {
	return nil
}

func (f FakePool) AddFilter(filter pool.Filter) {
}

func (f FakePool) Len() int {
	return 0
}

func (f FakePool) Add(transaction txn.Transaction) error {
	return nil
}

func (f FakePool) Remove(transaction txn.Transaction) error {
	return nil
}

func (f FakePool) Gather(ctx context.Context, config pool.Config) []txn.Transaction {
	return nil
}

func (f FakePool) Close() error {
	return nil
}

//
// Fake BlockStore
//

type FakeBlockStore struct {
}

func (f FakeBlockStore) Len() uint64 {
	return 0
}

func (f FakeBlockStore) Store(link orderingTypes.BlockLink) error {
	return nil
}

func (f FakeBlockStore) Get(id orderingTypes.Digest) (orderingTypes.BlockLink, error) {
	return nil, nil
}

func (f FakeBlockStore) GetByIndex(index uint64) (orderingTypes.BlockLink, error) {
	return nil, nil
}

func (f FakeBlockStore) GetChain() (orderingTypes.Chain, error) {
	return nil, nil
}

func (f FakeBlockStore) Last() (orderingTypes.BlockLink, error) {
	return nil, nil
}

func (f FakeBlockStore) Watch(ctx context.Context) <-chan orderingTypes.BlockLink {
	return nil
}

func (f FakeBlockStore) WithTx(transaction store.Transaction) blockstore.BlockStore {
	return nil
}
