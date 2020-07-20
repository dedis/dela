package mem

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/kv"
)

func TestDiskNode_Search(t *testing.T) {
	node := NewDiskNode(0)

	key := makeKey([]byte("ping"))
	bucket := newFakeBucket()

	leaf := NewLeafNode(0, key, []byte("pong"))
	err := node.store(key, leaf, bucket)
	require.NoError(t, err)

	value, err := node.Search(key, nil, bucket)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), value)
}

// Utility functions -----------------------------------------------------------

type fakeBucket struct {
	kv.Bucket
	store map[string][]byte
}

func newFakeBucket() *fakeBucket {
	return &fakeBucket{
		store: make(map[string][]byte),
	}
}

func (b *fakeBucket) Get(key []byte) []byte {
	return b.store[string(key)]
}

func (b *fakeBucket) Set(key, value []byte) error {
	b.store[string(key)] = value
	return nil
}
