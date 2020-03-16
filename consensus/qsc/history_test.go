package qsc

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

func TestEpoch_Pack(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		e := epoch{
			hash:   hash,
			random: random,
		}

		packed, err := e.Pack()
		require.NoError(t, err)
		require.Equal(t, hash, packed.(*Epoch).GetHash())
		require.Equal(t, random, packed.(*Epoch).GetRandom())

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestEpoch_Equal(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		e := epoch{
			hash:   hash,
			random: random,
		}

		require.True(t, e.Equal(e))
		require.False(t, e.Equal(epoch{
			hash:   hash,
			random: random + 1,
		}))
		require.False(t, e.Equal(epoch{
			random: random,
			hash:   append(append([]byte{}, hash...), 1),
		}))
		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistory_GetLast(t *testing.T) {
	h := history{}
	_, ok := h.getLast()
	require.False(t, ok)

	h = history{{}, {random: 1}}
	last, ok := h.getLast()
	require.True(t, ok)
	require.Equal(t, int64(1), last.random)
}

func TestHistory_Equal(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		h := history{epoch{hash: hash, random: random}}

		require.False(t, h.Equal(history{}))
		require.True(t, h.Equal(h))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistory_Pack(t *testing.T) {
	h := history{{}, {}, {}}
	packed, err := h.Pack()
	require.NoError(t, err)
	require.IsType(t, (*History)(nil), packed)
}

func TestHistory_String(t *testing.T) {
	h := history{}
	require.Equal(t, "History[0]{}", h.String())

	h = history{{}}
	require.Equal(t, "History[1]{nil}", h.String())

	h = history{{hash: []byte{0xaa, 0xbb}}}
	require.Equal(t, "History[1]{aabb}", h.String())
}
