package mem

import (
	"testing"

	"github.com/stretchr/testify/require"
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

	_, err = trie.Get([]byte("C"))
	require.EqualError(t, err, "item 0x43 not found")

	trie.store["D"] = item{deleted: true}
	_, err = trie.Get([]byte("D"))
	require.EqualError(t, err, "item 0x44 not found")
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
