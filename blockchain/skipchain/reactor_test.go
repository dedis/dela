package skipchain

import (
	fmt "fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestReactor_InvokeGenesis(t *testing.T) {
	expected := Digest{1}
	r := &reactor{
		operations: &operations{
			db: &fakeDatabase{
				blocks: []SkipBlock{{hash: expected}, {hash: Digest{2}}},
			},
		},
	}

	digest, err := r.InvokeGenesis()
	require.NoError(t, err)
	require.Equal(t, expected[:], digest)

	r.db = &fakeDatabase{}
	_, err = r.InvokeGenesis()
	require.EqualError(t, err,
		"couldn't read genesis block: block at index 0 not found")
}

func TestReactor_InvokeValidate(t *testing.T) {
	db := &fakeDatabase{blocks: []SkipBlock{{}}}
	r := &reactor{
		operations: &operations{
			reactor:      &fakeReactor{},
			watcher:      &fakeWatcher{},
			db:           db,
			addr:         fake.NewAddress(0),
			blockFactory: NewBlockFactory(fake.MessageFactory{}),
		},
		queue: &blockQueue{
			buffer: make(map[Digest]SkipBlock),
		},
	}

	req := Blueprint{
		index: 1,
		data:  fake.Message{},
	}

	hash, err := r.InvokeValidate(fake.Address{}, req)
	require.NoError(t, err)
	digest := Digest{}
	copy(digest[:], hash)
	block, ok := r.queue.Get(digest)
	require.True(t, ok)
	require.Equal(t, uint64(1), block.Index)

	_, err = r.InvokeValidate(fake.Address{}, fake.Message{})
	require.EqualError(t, err, "invalid message type 'fake.Message'")

	db.err = xerrors.New("oops")
	_, err = r.InvokeValidate(fake.Address{}, req)
	require.EqualError(t, err, "couldn't read genesis block: oops")

	db.err = nil
	db.blocks = []SkipBlock{{}}
	r.reactor = &fakeReactor{errValidate: xerrors.New("oops")}
	_, err = r.InvokeValidate(fake.Address{}, req)
	require.EqualError(t, err, "couldn't validate the payload: oops")

	r.reactor = &fakeReactor{}
	r.blockFactory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = r.InvokeValidate(fake.Address{}, req)
	require.EqualError(t, err,
		"couldn't compute hash: couldn't write index: fake error")

	req.index = 5
	r.rpc = fake.NewStreamRPC(fake.Receiver{}, fake.NewBadSender())
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
		queue: &blockQueue{buffer: make(map[Digest]SkipBlock)},
	}

	v.queue.Add(SkipBlock{hash: Digest{1, 2, 3}})
	v.queue.Add(SkipBlock{hash: Digest{1, 3}})
	err := v.InvokeCommit(Digest{1, 2, 3}.Bytes())
	require.NoError(t, err)
	require.Len(t, v.queue.buffer, 0)
	require.Equal(t, 1, watcher.notified)

	err = v.InvokeCommit([]byte{0xaa})
	require.Equal(t, 0, v.db.(*fakeDatabase).aborts)
	require.EqualError(t, err,
		fmt.Sprintf("couldn't find block '%v'", Digest{0xaa}))

	v.queue.Add(SkipBlock{hash: Digest{1, 2, 3}})
	v.db = &fakeDatabase{err: xerrors.New("oops")}
	err = v.InvokeCommit(Digest{1, 2, 3}.Bytes())
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
	blocks []SkipBlock
	err    error
	aborts int
}

func (db *fakeDatabase) Contains(index uint64) bool {
	return index < uint64(len(db.blocks))
}

func (db *fakeDatabase) Read(index int64) (SkipBlock, error) {
	if index >= int64(len(db.blocks)) {
		return SkipBlock{}, NewNoBlockError(index)
	}
	return db.blocks[index], db.err
}

func (db *fakeDatabase) Write(SkipBlock) error {
	return db.err
}

func (db *fakeDatabase) ReadLast() (SkipBlock, error) {
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
