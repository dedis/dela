package skipchain

import (
	fmt "fmt"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestBlockValidator_Validate(t *testing.T) {
	f := func(block SkipBlock) bool {
		packed, err := block.Pack(encoding.NewProtoEncoder())
		require.NoError(t, err)

		db := &fakeDatabase{blocks: []SkipBlock{{hash: block.GenesisID}}}
		v := &blockValidator{
			operations: &operations{
				processor: &fakePayloadProc{},
				watcher:   &fakeWatcher{},
				encoder:   encoding.NewProtoEncoder(),
				db:        db,
				addr:      fake.NewAddress(0),
				blockFactory: blockFactory{
					encoder:     encoding.NewProtoEncoder(),
					hashFactory: crypto.NewSha256Factory(),
				},
			},
			queue: &blockQueue{
				buffer: make(map[Digest]SkipBlock),
			},
		}
		prop, err := v.Validate(fake.Address{}, packed)
		require.NoError(t, err)
		require.NotNil(t, prop)
		require.Equal(t, block.GetHash(), prop.GetHash())
		require.Equal(t, block.BackLink.Bytes(), prop.GetPreviousHash())

		_, err = v.Validate(fake.Address{}, nil)
		require.EqualError(t, err, "couldn't decode block: invalid message type '<nil>'")

		db.err = xerrors.New("oops")
		_, err = v.Validate(fake.Address{}, packed)
		require.EqualError(t, err, "couldn't read genesis block: oops")

		db.err = nil
		db.blocks = []SkipBlock{{}}
		_, err = v.Validate(fake.Address{}, packed)
		require.EqualError(t, err,
			fmt.Sprintf("mismatch genesis hash '%v' != '%v'", Digest{}, block.GenesisID))

		db.blocks = []SkipBlock{{hash: block.GenesisID}}
		v.processor = &fakePayloadProc{errValidate: xerrors.New("oops")}
		_, err = v.Validate(fake.Address{}, packed)
		require.EqualError(t, err, "couldn't validate the payload: oops")

		v.db = &fakeDatabase{missing: true}
		v.rpc = fake.NewStreamRPC(fake.Receiver{}, fake.NewBadSender())
		_, err = v.Validate(fake.Address{}, packed)
		require.EqualError(t, err,
			"couldn't catch up: couldn't send block request: fake error")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockValidator_Commit(t *testing.T) {
	watcher := &fakeWatcher{}
	v := &blockValidator{
		operations: &operations{
			processor: &fakePayloadProc{},
			watcher:   watcher,
			db:        &fakeDatabase{},
		},
		queue: &blockQueue{buffer: make(map[Digest]SkipBlock)},
	}

	v.queue.Add(SkipBlock{hash: Digest{1, 2, 3}})
	v.queue.Add(SkipBlock{hash: Digest{1, 3}})
	err := v.Commit(Digest{1, 2, 3}.Bytes())
	require.NoError(t, err)
	require.Len(t, v.queue.buffer, 0)
	require.Equal(t, 1, watcher.notified)

	err = v.Commit([]byte{0xaa})
	require.Equal(t, 0, v.db.(*fakeDatabase).aborts)
	require.EqualError(t, err,
		fmt.Sprintf("couldn't find block '%v'", Digest{0xaa}))

	v.queue.Add(SkipBlock{hash: Digest{1, 2, 3}})
	v.db = &fakeDatabase{err: xerrors.New("oops")}
	err = v.Commit(Digest{1, 2, 3}.Bytes())
	require.EqualError(t, err, "couldn't commit block: tx failed: couldn't write block: oops")
	require.Equal(t, 1, v.db.(*fakeDatabase).aborts)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePayloadProc struct {
	blockchain.PayloadProcessor
	calls       [][]interface{}
	errValidate error
	errCommit   error
}

func (v *fakePayloadProc) Validate(index uint64, data proto.Message) error {
	v.calls = append(v.calls, []interface{}{index, data})
	return v.errValidate
}

func (v *fakePayloadProc) Commit(data proto.Message) error {
	v.calls = append(v.calls, []interface{}{data})
	return v.errCommit
}

type fakeDatabase struct {
	Database
	blocks  []SkipBlock
	err     error
	aborts  int
	missing bool
}

func (db *fakeDatabase) Contains(index uint64) bool {
	return !db.missing
}

func (db *fakeDatabase) Read(index int64) (SkipBlock, error) {
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
}

func (w *fakeWatcher) Notify(event interface{}) {
	w.notified++
}

func (w *fakeWatcher) Add(obs blockchain.Observer) {
	w.count++
}

func (w *fakeWatcher) Remove(obs blockchain.Observer) {
	w.count--
}
