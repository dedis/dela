package qsc

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
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

func TestHistories_Decode(t *testing.T) {
	h1, h1a := makeHistory(t, 1)
	h2, h2a := makeHistory(t, 2)
	h3, h3a := makeHistory(t, 3)

	ms := map[int64]*Message{
		1:  {Node: 1, Value: h1a},
		2:  {Node: 2, Value: h2a},
		30: {Node: 30, Value: h3a},
	}

	hists, err := decodeHistories(ms)
	require.NoError(t, err)
	require.Len(t, hists, 3)
	for _, history := range hists {
		if len(history) == 1 {
			require.Equal(t, h1, history)
		} else if len(history) == 2 {
			require.Equal(t, h2, history)
		} else {
			require.Equal(t, h3, history)
		}
	}

	ms = map[int64]*Message{
		1: {},
	}
	_, err = decodeHistories(ms)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*History)(nil), nil)))
}

func TestHistories_GetBest(t *testing.T) {
	f := func(r1, r2 int64) bool {
		h1 := history{{random: r1}}
		h2 := history{{random: r2}}
		hists := histories{h1, h2}

		if r2 > r1 {
			require.Equal(t, h2, hists.getBest())
		} else {
			require.Equal(t, h1, hists.getBest())
		}

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	hists := histories{}
	best := hists.getBest()
	require.Nil(t, best)
}

func TestHistories_Contains(t *testing.T) {
	f := func() bool {
		h1, _ := makeHistory(t, 3)
		h2, _ := makeHistory(t, 5)
		h3, _ := makeHistory(t, 0)
		h4, _ := makeHistory(t, 1)
		hists := histories{h1, h2, {}}

		return hists.contains(h1) && hists.contains(h2) &&
			!hists.contains(h3) && !hists.contains(h4)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistories_IsUniqueBest(t *testing.T) {
	f := func(r int64) bool {
		h1 := history{{random: r}}
		h2 := history{{random: r + 1}}
		h3 := history{{}, {random: r}}

		hists := histories{h1, h2, {}}
		require.True(t, hists.isUniqueBest(h2))
		require.False(t, hists.isUniqueBest(h1))

		hists = histories{h1, h3}
		require.False(t, hists.isUniqueBest(h1))

		require.False(t, hists.isUniqueBest(history{}))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func makeHistory(t *testing.T, n int) (history, *any.Any) {
	epochs := make([]epoch, n)
	for i := range epochs {
		epochs[i] = epoch{
			random: rand.Int63(),
			hash:   []byte{0xaa},
		}
	}

	h := history(epochs)
	pb, err := h.Pack()
	require.NoError(t, err)
	pbany, err := ptypes.MarshalAny(pb)
	require.NoError(t, err)

	return h, pbany
}
