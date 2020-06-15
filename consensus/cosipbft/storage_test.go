package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
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

func TestStorage_StoreChain(t *testing.T) {
	storage := newInMemoryStorage()

	err := storage.StoreChain(makeChain(0xa, 0xb))
	require.NoError(t, err)

	err = storage.StoreChain(makeChain(0xa, 0xb, 0xc, 0xd))
	require.NoError(t, err)

	err = storage.StoreChain((consensus.Chain)(nil))
	require.EqualError(t, err, "invalid chain type '<nil>'")

	err = storage.StoreChain(makeChain(0xa, 0xb, 0xd))
	require.EqualError(t, err, "mismatch link 1: to '0x0d' != '0x0c'")

	err = storage.StoreChain(makeChain(0xb, 0xc))
	require.EqualError(t, err, "mismatch link 0: from '0x0b' != '0x0a'")

	chain := makeChain(0xa, 0xb, 0xc, 0xd, 0xe)
	chain.links[3].from = []byte{0xf}
	err = storage.StoreChain(chain)
	require.EqualError(t, err, "couldn't append link: '0x0d' != '0x0f'")
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

// -----------------------------------------------------------------------------
// Utility functions

func makeChain(digests ...byte) forwardLinkChain {
	links := make([]forwardLink, len(digests)-1)
	for i := range links {
		links[i].from = []byte{digests[i]}
		links[i].to = []byte{digests[i+1]}
	}

	return forwardLinkChain{links: links}
}
