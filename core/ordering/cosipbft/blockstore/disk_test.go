package blockstore

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestInDisk_Len(t *testing.T) {
	store := NewDiskStore(nil, nil)
	require.Equal(t, uint64(0), store.Len())

	store.length = 5
	require.Equal(t, uint64(5), store.Len())
}

func TestInDisk_Load(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	err := store.Load()
	require.NoError(t, err)

	err = store.Store(makeLink(t, types.Digest{}))
	require.NoError(t, err)

	err = store.Store(makeLink(t, store.last.GetTo()))
	require.NoError(t, err)

	newStore := NewDiskStore(db, makeBlockFac())

	err = newStore.Load()
	require.NoError(t, err)

	store.fac = badLinkFac{}
	err = store.Load()
	require.EqualError(t, err, fake.Err("while scanning: malformed block"))
}

func TestInDisk_Store(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	err := store.Store(makeLink(t, types.Digest{}, types.WithIndex(0)))
	require.NoError(t, err)
	require.Equal(t, uint64(1), store.length)
	require.NotNil(t, store.last)
	require.Len(t, store.indices, 1)

	err = store.Store(makeLink(t, store.last.GetTo(), types.WithIndex(1)))
	require.NoError(t, err)
	require.Equal(t, uint64(2), store.length)
	require.Len(t, store.indices, 2)

	err = store.Store(makeLink(t, types.Digest{}))
	require.EqualError(t, err, "mismatch digests '00000000' (new) != 'b68f5931' (last)")

	store = NewDiskStore(db, makeBlockFac())
	err = store.Store(badLink{})
	require.EqualError(t, err, fake.Err("failed to serialize"))

	store.db = badDB{}
	err = store.Store(makeLink(t, types.Digest{}))
	require.EqualError(t, err, fake.Err("bucket failed"))

	store.db = badDB{bucket: badBucket{}}
	err = store.Store(makeLink(t, types.Digest{}))
	require.EqualError(t, err, fake.Err("while writing"))
}

func TestInDisk_Get(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	link, err := store.Get(types.Digest{})
	require.EqualError(t, err, "'00000000' not found: no block")
	require.Nil(t, link)

	err = store.Store(makeLink(t, types.Digest{}, types.WithIndex(2)))
	require.NoError(t, err)

	link, err = store.Get(store.last.GetTo())
	require.NoError(t, err)
	require.Equal(t, uint64(2), link.GetBlock().GetIndex())
}

func TestInDisk_GetByIndex(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	err := store.Store(makeLink(t, types.Digest{}, types.WithIndex(0)))
	require.NoError(t, err)

	err = store.Store(makeLink(t, store.last.GetTo(), types.WithIndex(1)))
	require.NoError(t, err)

	link, err := store.GetByIndex(1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), link.GetBlock().GetIndex())

	_, err = store.GetByIndex(3)
	require.EqualError(t, err, "index 3 not found: no block")

	store.fac = badLinkFac{}
	_, err = store.GetByIndex(0)
	require.EqualError(t, err, fake.Err("malformed block"))
}

func TestInDisk_GetChain(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	_, err := store.GetChain()
	require.EqualError(t, err, "store is empty")

	err = store.Store(makeLink(t, types.Digest{}, types.WithIndex(0)))
	require.NoError(t, err)

	err = store.Store(makeLink(t, store.last.GetTo(), types.WithIndex(1)))
	require.NoError(t, err)

	chain, err := store.GetChain()
	require.NoError(t, err)
	require.Equal(t, uint64(1), chain.GetBlock().GetIndex())

	store.fac = badLinkFac{}
	_, err = store.GetChain()
	require.EqualError(t, err, fake.Err("while reading database: while scanning: block malformed"))
}

func TestInDisk_Last(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	last, err := store.Last()
	require.EqualError(t, err, "store is empty: no block")
	require.Nil(t, last)

	store.length = 1
	store.last = makeLink(t, types.Digest{})
	last, err = store.Last()
	require.NoError(t, err)
	require.NotNil(t, last)
}

func TestInDisk_Watch(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	links := store.Watch(ctx)

	err := store.Store(makeLink(t, types.Digest{}, types.WithIndex(2)))
	require.NoError(t, err)

	link := <-links
	require.Equal(t, uint64(2), link.GetBlock().GetIndex())

	cancel()
	_, more := <-links
	require.False(t, more)
}

func TestInDisk_WithTx(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	store := NewDiskStore(db, makeBlockFac())

	err := db.Update(func(tx kv.WritableTx) error {
		next := store.WithTx(tx)
		err := next.Store(makeLink(t, types.Digest{}))
		require.NoError(t, err)

		link, err := next.GetByIndex(0)
		require.NoError(t, err)
		require.NotNil(t, link)

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), store.length)

	next := NewDiskStore(db, makeBlockFac()).WithTx(dummyTx{})

	err = next.Store(makeLink(t, types.Digest{}))
	require.EqualError(t, err, "transaction 'blockstore.dummyTx' is not writable")

	_, err = next.GetByIndex(0)
	require.EqualError(t, err, "transaction 'blockstore.dummyTx' is not readable")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeDB(t *testing.T) (kv.DB, func()) {
	file, err := os.CreateTemp(os.TempDir(), "dela-blockstore")
	require.NoError(t, err)

	db, err := kv.New(file.Name())
	require.NoError(t, err)

	clean := func() {
		db.Close()
		file.Close()
	}

	return db, clean
}

func makeBlockFac() types.LinkFactory {
	blockFac := types.NewBlockFactory(fakeResultFac{})

	return types.NewLinkFactory(blockFac, fake.SignatureFactory{}, fakeCsFac{})
}

type fakeCsFac struct {
	authority.ChangeSetFactory
}

func (fakeCsFac) ChangeSetOf(serde.Context, []byte) (authority.ChangeSet, error) {
	return authority.NewChangeSet(), nil
}

type fakeResultFac struct {
	validation.ResultFactory
}

func (fakeResultFac) ResultOf(serde.Context, []byte) (validation.Result, error) {
	return simple.NewResult(nil), nil
}

type badLinkFac struct {
	types.LinkFactory
}

func (badLinkFac) BlockLinkOf(serde.Context, []byte) (types.BlockLink, error) {
	return nil, fake.GetError()
}

type badLink struct {
	types.BlockLink
}

func (badLink) GetFrom() types.Digest {
	return types.Digest{}
}

func (badLink) Serialize(serde.Context) ([]byte, error) {
	return nil, fake.GetError()
}

type dummyTx struct {
	store.Transaction
}
