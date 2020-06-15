package qsc

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

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

	h = history{epochs: []epoch{{}, {random: 1}}}
	last, ok := h.getLast()
	require.True(t, ok)
	require.Equal(t, int64(1), last.random)
}

func TestHistory_Equal(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		h := history{epochs: []epoch{{hash: hash, random: random}}}

		require.False(t, h.Equal(history{}))
		require.True(t, h.Equal(h))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistory_String(t *testing.T) {
	h := history{}
	require.Equal(t, "History[0]{}", h.String())

	h = history{epochs: []epoch{{}}}
	require.Equal(t, "History[1]{nil}", h.String())

	h = history{epochs: []epoch{{hash: []byte{0xaa, 0xbb}}}}
	require.Equal(t, "History[1]{aabb}", h.String())
}

func TestHistoryFactory_VisitJSON(t *testing.T) {
	factory := HistoryFactory{}

	ser := json.NewSerializer()

	var hist history
	err := ser.Deserialize([]byte(`[{},{}]`), factory, &hist)
	require.NoError(t, err)
	require.Len(t, hist.epochs, 2)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}

func TestHistories_GetBest(t *testing.T) {
	f := func(r1, r2 int64) bool {
		h1 := history{epochs: []epoch{{random: r1}}}
		h2 := history{epochs: []epoch{{random: r2}}}
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
	require.Equal(t, history{}, best)
}

func TestHistories_Contains(t *testing.T) {
	f := func() bool {
		h1 := makeHistory(3)
		h2 := makeHistory(5)
		h3 := makeHistory(0)
		h4 := makeHistory(1)
		hists := histories{h1, h2, {}}

		return hists.contains(h1) && hists.contains(h2) &&
			!hists.contains(h3) && !hists.contains(h4)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistories_IsUniqueBest(t *testing.T) {
	f := func(r int64) bool {
		h1 := history{epochs: []epoch{{random: r}}}
		h2 := history{epochs: []epoch{{random: r + 1}}}
		h3 := history{epochs: []epoch{{}, {random: r}}}

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

func TestHistoriesFactory_FromMessageSet(t *testing.T) {
	h1 := makeHistory(1)
	h2 := makeHistory(2)
	h3 := makeHistory(3)

	ms := map[int64]Message{
		1:  {node: 1, value: h1},
		2:  {node: 2, value: h2},
		30: {node: 30, value: h3},
	}

	factory := defaultHistoriesFactory{}

	hists, err := factory.FromMessageSet(ms)
	require.NoError(t, err)
	require.Len(t, hists, 3)
	for _, history := range hists {
		if len(history.epochs) == 1 {
			require.Equal(t, h1, history)
		} else if len(history.epochs) == 2 {
			require.Equal(t, h2, history)
		} else {
			require.Equal(t, h3, history)
		}
	}
}

// -----------------------------------------------------------------------------
// Utility functions

func makeHistory(n int) history {
	epochs := make([]epoch, n)
	for i := range epochs {
		epochs[i] = epoch{
			random: rand.Int63(),
			hash:   []byte{0xaa},
		}
	}

	h := history{epochs: epochs}

	return h
}
