// Package binprefix implements the hash tree interface by following the merkle
// binary prefix tree algorithm.
//
// https://www.usenix.org/system/files/conference/usenixsecurity15/sec15-paper-melara.pdf
//
// The merkle tree is stored in-memory until it reaches a certain threshold of
// depth where it will write the nodes in disk. The leaf are always stored in
// disk because of the value it holds.
//
//                         Interior (Root)
//                           /        \
//                        0 /          \ 1
//                         /            \
//                      Interior       Interior
//                       /    \          /   \
//                    0 /      \ 1    0 /     \ 1
//                     /        \      /       \
//                DiskNode  Interior  Empty    Interior
//                           /   \              /    \
// -------------------------------------------------------------- Memory Depth
//                       0 /       \ 1      0 /        \ 1
//                        /         \        /          \
//                    DiskNode  DiskNode  DiskNode   DiskNode
//
// The drawing above demonstrates an example of a tree. Here the memory depth is
// set at 3 which means that every node after this level will be a disk node. It
// will be loaded to its in-memory type when traversing the tree. The node at
// prefix 00 is an example of a leaf node which is a disk node even above the
// memory depth level.
//
// Documentation Last Review: 08.10.2020
//
package binprefix

import (
	"sync"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// MerkleTree is an implementation of a Merkle prefix binary tree. This
// particular implementation assumes the keys will have the same length so that
// only the longest unique prefix along the path can be a leaf node.
//
// The leafs of the tree will be stored on the disk when committing the tree.
// Modifications on a staged tree are done in-memory.
//
// - implements hashtree.Tree
type MerkleTree struct {
	sync.Mutex

	tree        *Tree
	db          kv.DB
	tx          store.Transaction
	bucket      []byte
	hashFactory crypto.HashFactory
}

// NewMerkleTree creates a new Merkle tree-based storage.
func NewMerkleTree(db kv.DB, nonce Nonce) *MerkleTree {
	return &MerkleTree{
		tree:        NewTree(nonce),
		db:          db,
		bucket:      []byte("hashtree"),
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Load tries to read the bucket and scan it for existing leafs and populate the
// tree with them.
func (t *MerkleTree) Load() error {
	t.Lock()
	defer t.Unlock()

	return t.doUpdate(func(tx kv.WritableTx) error {
		bucket := tx.GetBucket(t.bucket)

		err := t.tree.FillFromBucket(bucket)
		if err != nil {
			return xerrors.Errorf("failed to load: %v", err)
		}

		err = t.tree.CalculateRoot(t.hashFactory, bucket)
		if err != nil {
			return xerrors.Errorf("while updating: %v", err)
		}

		return nil
	})
}

// Get implements store.Readable. It returns the value associated with the key
// if it exists, otherwise it returns nil.
func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	t.Lock()
	defer t.Unlock()

	var value []byte

	err := t.doView(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(t.bucket)

		var err error
		value, err = t.tree.Search(key, nil, bucket)

		return err
	})

	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return value, nil
}

// GetRoot implements hashtree.Tree. It returns the root hash of the tree.
func (t *MerkleTree) GetRoot() []byte {
	t.Lock()
	defer t.Unlock()

	return t.tree.root.GetHash()
}

// GetPath implements hashtree.Tree. It returns a path to a given key that can
// be used to prove the inclusion or the absence of a key.
func (t *MerkleTree) GetPath(key []byte) (hashtree.Path, error) {
	t.Lock()
	defer t.Unlock()

	path := newPath(t.tree.nonce[:], key)

	err := t.doView(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(t.bucket)

		_, err := t.tree.Search(key, &path, bucket)
		return err
	})

	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return path, nil
}

// Stage implements hashtree.Tree. It executes the callback over a clone of the
// current tree and return the clone with the root calculated.
func (t *MerkleTree) Stage(fn func(store.Snapshot) error) (hashtree.StagingTree, error) {
	clone := t.clone()

	err := t.doUpdate(func(tx kv.WritableTx) error {
		b, err := tx.GetBucketOrCreate(t.bucket)
		if err != nil {
			return xerrors.Errorf("read bucket failed: %v", err)
		}

		err = fn(writableMerkleTree{
			MerkleTree: clone,
			bucket:     b,
		})

		if err != nil {
			return xerrors.Errorf("callback failed: %v", err)
		}

		err = clone.tree.CalculateRoot(t.hashFactory, b)
		if err != nil {
			return xerrors.Errorf("couldn't update tree: %v", err)
		}

		return nil
	})

	return clone, err
}

// Commit implements hashtree.StagingTree. It writes the leaf nodes to the disk
// and a trade-off of other nodes.
func (t *MerkleTree) Commit() error {
	t.Lock()
	defer t.Unlock()

	err := t.doUpdate(func(tx kv.WritableTx) error {
		bucket, err := tx.GetBucketOrCreate(t.bucket)
		if err != nil {
			return xerrors.Errorf("read bucket failed: %v", err)
		}

		return t.tree.Persist(bucket)
	})

	if err != nil {
		return xerrors.Errorf("failed to persist tree: %v", err)
	}

	return nil
}

// WithTx implements hashtree.StagingTree. It returns a tree that will share the
// same underlying data but it will perform operations on the database through
// the transaction.
func (t *MerkleTree) WithTx(tx store.Transaction) hashtree.StagingTree {
	return &MerkleTree{
		tree:        t.tree,
		db:          t.db,
		tx:          tx,
		bucket:      t.bucket,
		hashFactory: t.hashFactory,
	}
}

func (t *MerkleTree) clone() *MerkleTree {
	return &MerkleTree{
		tree:        t.tree.Clone(),
		db:          t.db,
		tx:          t.tx,
		bucket:      t.bucket,
		hashFactory: t.hashFactory,
	}
}

func (t *MerkleTree) doUpdate(fn func(kv.WritableTx) error) error {
	if t.tx != nil {
		tx, ok := t.tx.(kv.WritableTx)
		if !ok {
			return xerrors.Errorf("transaction '%T' is not writable", t.tx)
		}

		return fn(tx)
	}

	return t.db.Update(fn)
}

func (t *MerkleTree) doView(fn func(kv.ReadableTx) error) error {
	if t.tx != nil {
		tx, ok := t.tx.(kv.ReadableTx)
		if !ok {
			return xerrors.Errorf("transaction '%T' is not readable", t.tx)
		}

		return fn(tx)
	}

	return t.db.View(fn)
}

// WritableMerkleTree is a wrapper around the merkle tree implementation so that
// it can be written into but the tree is not updated at every operation.
//
// - implements store.Writable
type writableMerkleTree struct {
	*MerkleTree

	bucket kv.Bucket
}

// Set implements store.Writable. It adds or updates the key in the internal
// tree.
func (t writableMerkleTree) Set(key, value []byte) error {
	t.Lock()
	defer t.Unlock()

	err := t.tree.Insert(key, value, t.bucket)
	if err != nil {
		return xerrors.Errorf("couldn't insert pair: %v", err)
	}

	return nil
}

// Delete implements store.Writable. It removes the key from the tree.
func (t writableMerkleTree) Delete(key []byte) error {
	t.Lock()
	defer t.Unlock()

	err := t.tree.Delete(key, t.bucket)
	if err != nil {
		return xerrors.Errorf("couldn't delete key: %v", err)
	}

	return nil
}
