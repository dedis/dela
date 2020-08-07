package skipchain

import (
	fmt "fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/blockchain/skipchain/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestReactor_InvokeGenesis(t *testing.T) {
	expected := makeBlock(t, types.WithIndex(1))
	r := &reactor{
		operations: &operations{
			db: &fakeDatabase{
				blocks: []types.SkipBlock{expected, makeBlock(t, types.WithIndex(2))},
			},
		},
	}

	digest, err := r.InvokeGenesis()
	require.NoError(t, err)
	require.Equal(t, expected.GetHash(), digest)

	r.db = &fakeDatabase{}
	_, err = r.InvokeGenesis()
	require.EqualError(t, err,
		"couldn't read genesis block: block at index 0 not found")
}

func TestReactor_InvokeValidate(t *testing.T) {
	db := &fakeDatabase{blocks: []types.SkipBlock{{}}}
	r := &reactor{
		operations: &operations{
			reactor: &fakeReactor{},
			watcher: &fakeWatcher{},
			db:      db,
			addr:    fake.NewAddress(0),
		},
		queue: types.NewQueue(),
	}

	req := types.NewBlueprint(1, nil, fake.Message{})

	hash, err := r.InvokeValidate(fake.Address{}, req)
	require.NoError(t, err)

	block, ok := r.queue.Get(hash)
	require.True(t, ok)
	require.Equal(t, uint64(1), block.Index)

	_, err = r.InvokeValidate(fake.Address{}, fake.Message{})
	require.EqualError(t, err, "invalid message type 'fake.Message'")

	db.err = xerrors.New("oops")
	_, err = r.InvokeValidate(fake.Address{}, req)
	require.EqualError(t, err, "couldn't read genesis block: oops")

	db.err = nil
	db.blocks = []types.SkipBlock{{}}
	r.reactor = &fakeReactor{errValidate: xerrors.New("oops")}
	_, err = r.InvokeValidate(fake.Address{}, req)
	require.EqualError(t, err, "couldn't validate the payload: oops")

	r.reactor = &fakeReactor{}
	req = types.NewBlueprint(5, nil, nil)
	r.rpc = fake.NewStreamRPC(fake.NewReceiver(), fake.NewBadSender())
	_, err = r.InvokeValidate(fake.Address{}, req)
	require.EqualError(t, err,
		"couldn't catch up: couldn't send block request: fake error")
}

func TestReactor_InvokeCommit(t *testing.T) {
	watcher := &fakeWatcher{}
	v := &reactor{
		operations: &operations{
			reactor: &fakeReactor{},
			watcher: watcher,
			db:      &fakeDatabase{},
		},
		queue: types.NewQueue(),
	}

	block1 := makeBlock(t, types.WithIndex(0))
	block2 := makeBlock(t, types.WithIndex(1))

	v.queue.Add(block1)
	v.queue.Add(block2)
	err := v.InvokeCommit(block1.GetHash())
	require.NoError(t, err)
	require.Equal(t, 0, v.queue.Len())
	require.Equal(t, 1, watcher.notified)

	err = v.InvokeCommit([]byte{0xaa})
	require.Equal(t, 0, v.db.(*fakeDatabase).aborts)
	require.EqualError(t, err,
		fmt.Sprintf("couldn't find block %#x", []byte{0xaa}))

	v.queue.Add(block1)
	v.db = &fakeDatabase{err: xerrors.New("oops")}
	err = v.InvokeCommit(block1.GetHash())
	require.EqualError(t, err, "couldn't commit block: tx failed: couldn't write block: oops")
	require.Equal(t, 1, v.db.(*fakeDatabase).aborts)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeReactor struct {
	blockchain.Reactor

	calls       [][]interface{}
	errValidate error
	errCommit   error
}

func (v *fakeReactor) InvokeValidate(data serde.Message) (blockchain.Payload, error) {
	v.calls = append(v.calls, []interface{}{data})
	return fake.Message{}, v.errValidate
}

func (v *fakeReactor) InvokeCommit(p blockchain.Payload) error {
	v.calls = append(v.calls, []interface{}{p})
	return v.errCommit
}

type fakeDatabase struct {
	Database
	blocks []types.SkipBlock
	err    error
	aborts int
}

func (db *fakeDatabase) Contains(index uint64) bool {
	return index < uint64(len(db.blocks))
}

func (db *fakeDatabase) Read(index int64) (types.SkipBlock, error) {
	if index >= int64(len(db.blocks)) {
		return types.SkipBlock{}, NewNoBlockError(index)
	}
	return db.blocks[index], db.err
}

func (db *fakeDatabase) Write(types.SkipBlock) error {
	return db.err
}

func (db *fakeDatabase) ReadLast() (types.SkipBlock, error) {
	return db.blocks[len(db.blocks)-1], db.err
}

func (db *fakeDatabase) Atomic(tx func(Queries) error) error {
	err := tx(db)
	if err != nil {
		db.aborts++
	}
	return err
}

type fakeWatcher struct {
	blockchain.Observable
	count    int
	notified int
	block    blockchain.Block
	call     *fake.Call
}

func (w *fakeWatcher) Notify(event interface{}) {
	w.notified++
}

func (w *fakeWatcher) Add(obs blockchain.Observer) {
	w.call.Add(obs)
	w.count++
	if w.block != nil {
		obs.NotifyCallback(w.block)
	}
}

func (w *fakeWatcher) Remove(obs blockchain.Observer) {
	w.call.Add(obs)
	w.count--
}
