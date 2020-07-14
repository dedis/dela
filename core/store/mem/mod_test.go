package mem

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestTrie_Get(t *testing.T) {
	trie := NewTrie()

	trie.store["A"] = item{value: []byte{1}}
	trie.parent = NewTrie()
	trie.parent.store["B"] = item{value: []byte{2}}
	trie.parent.store["D"] = item{value: []byte{3}}

	value, err := trie.Get([]byte("A"))
	require.NoError(t, err)
	require.Equal(t, []byte{1}, value)

	value, err = trie.Get([]byte("B"))
	require.NoError(t, err)
	require.Equal(t, []byte{2}, value)

	value, err = trie.Get([]byte("C"))
	require.NoError(t, err)
	require.Nil(t, value)

	trie.store["D"] = item{deleted: true}
	value, err = trie.Get([]byte("D"))
	require.NoError(t, err)
	require.Nil(t, value)
}

func TestTrie_Set(t *testing.T) {
	trie := NewTrie()

	trie.Set([]byte("A"), []byte{1})
	require.Equal(t, item{value: []byte{1}}, trie.store["A"])
}

func TestTrie_Delete(t *testing.T) {
	trie := NewTrie()
	trie.store["A"] = item{value: []byte{1}}

	require.NoError(t, trie.Delete([]byte("A")))
	require.Equal(t, item{deleted: true}, trie.store["A"])

	require.NoError(t, trie.Delete([]byte("B")))
	require.Equal(t, item{deleted: true}, trie.store["B"])
}

func TestTrie_GetRoot(t *testing.T) {
	trie := NewTrie()

	require.Nil(t, trie.GetRoot())

	trie.root = []byte{1}
	require.Equal(t, []byte{1}, trie.GetRoot())
}

func TestTrie_GetShare(t *testing.T) {
	trie := NewTrie()
	trie.store["B"] = item{value: []byte{1}}
	trie.root = []byte{3}

	share, err := trie.GetShare([]byte("A"))
	require.NoError(t, err)
	require.Nil(t, share.GetValue())

	share, err = trie.GetShare([]byte("B"))
	require.NoError(t, err)
	require.Equal(t, []byte("B"), share.GetKey())
	require.Equal(t, []byte{1}, share.GetValue())
	require.Equal(t, []byte{3}, share.GetRoot())
}

func TestTrie_Fingerprint(t *testing.T) {
	trie := NewTrie()
	trie.store["A"] = item{value: []byte{1}}
	trie.store["B"] = item{deleted: true}
	trie.parent = NewTrie()
	trie.parent.root = []byte{2}

	buffer := new(bytes.Buffer)
	err := trie.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x02A\x01", buffer.String())

	err = trie.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write parent root: fake error")

	err = trie.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write key: fake error")

	err = trie.Fingerprint(fake.NewBadHashWithDelay(2))
	require.EqualError(t, err, "couldn't write value: fake error")
}

func TestTrie_Stage(t *testing.T) {
	trie := NewTrie()

	next, err := trie.Stage(func(rwt store.ReadWriteTrie) error {
		require.NoError(t, rwt.Set([]byte("A"), []byte{1}))
		require.NoError(t, rwt.Set([]byte("B"), []byte{2}))
		return nil
	})

	require.NoError(t, err)
	// Check the SHA-256 root is calculated.
	require.Len(t, next.GetRoot(), 32)
	require.Len(t, next.(*Trie).store, 2)

	_, err = trie.Stage(func(store.ReadWriteTrie) error {
		return xerrors.New("oops")
	})
	require.EqualError(t, err, "callback failed: oops")

	trie.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = trie.Stage(func(rwt store.ReadWriteTrie) error { return nil })
	require.EqualError(t, err,
		"couldn't compute root: couldn't write parent root: fake error")
}
