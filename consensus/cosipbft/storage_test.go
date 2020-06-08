package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorage_Store(t *testing.T) {
	storage := newInMemoryStorage()

	err := storage.Store(forwardLink{
		from: []byte{0xaa},
		to:   []byte{0xbb},
	})
	require.NoError(t, err)
	require.Len(t, storage.links, 1)

	err = storage.Store(forwardLink{
		from: []byte{0xbb},
		to:   []byte{0xcc},
	})
	require.NoError(t, err)
	require.Len(t, storage.links, 2)

	err = storage.Store(forwardLink{from: []byte{0xff}})
	require.EqualError(t, err, "mismatch forward link 'cc' != 'ff'")
}

func TestStorage_ReadChain(t *testing.T) {
	storage := newInMemoryStorage()
	storage.links = []forwardLink{
		{to: []byte{0xaa}},
		{to: []byte{0xbb}},
	}

	chain, err := storage.ReadChain([]byte{0xaa})
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 1)

	chain, err = storage.ReadChain([]byte{0xbb})
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 2)

	_, err = storage.ReadChain([]byte{0xcc})
	require.EqualError(t, err, "id 'cc' not found")
}
