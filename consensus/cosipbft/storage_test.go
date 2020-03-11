package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorage_Store(t *testing.T) {
	storage := newInMemoryStorage()

	err := storage.Store(&ForwardLinkProto{
		From: []byte{0xaa},
		To:   []byte{0xbb},
	})
	require.NoError(t, err)
	require.Len(t, storage.links, 1)

	err = storage.Store(&ForwardLinkProto{
		From: []byte{0xbb},
		To:   []byte{0xcc},
	})
	require.NoError(t, err)
	require.Len(t, storage.links, 2)

	err = storage.Store(&ForwardLinkProto{From: []byte{0xff}})
	require.EqualError(t, err, "mismatch forward link 'cc' != 'ff'")
}

func TestStorage_ReadLast(t *testing.T) {
	storage := newInMemoryStorage()

	fl, err := storage.ReadLast()
	require.NoError(t, err)
	require.Nil(t, fl)

	storage.links = []*ForwardLinkProto{{From: []byte{0xaa}}}
	fl, err = storage.ReadLast()
	require.NoError(t, err)
	require.Equal(t, []byte{0xaa}, fl.GetFrom())

	storage.links = []*ForwardLinkProto{
		{From: []byte{0xaa}},
		{From: []byte{0xbb}},
	}
	fl, err = storage.ReadLast()
	require.NoError(t, err)
	require.Equal(t, []byte{0xbb}, fl.GetFrom())
}

func TestStorage_ReadChain(t *testing.T) {
	storage := newInMemoryStorage()
	storage.links = []*ForwardLinkProto{
		{To: []byte{0xaa}},
		{To: []byte{0xbb}},
	}

	chain, err := storage.ReadChain([]byte{0xaa})
	require.NoError(t, err)
	require.Len(t, chain, 1)

	chain, err = storage.ReadChain([]byte{0xbb})
	require.NoError(t, err)
	require.Len(t, chain, 2)

	chain, err = storage.ReadChain([]byte{0xcc})
	require.EqualError(t, err, "id 'cc' not found")
}
