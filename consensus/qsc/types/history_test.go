package types

import (
	"bytes"
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterHistoryFormat(fake.GoodFormat, fake.Format{Msg: History{}})
	RegisterHistoryFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestEpoch_Getters(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		epoch := NewEpoch(hash, random)

		return bytes.Equal(hash, epoch.GetHash()) && random == epoch.GetRandom()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestEpoch_Equal(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		e := Epoch{
			hash:   hash,
			random: random,
		}

		require.True(t, e.Equal(e))
		require.False(t, e.Equal(Epoch{
			hash:   hash,
			random: random + 1,
		}))
		require.False(t, e.Equal(Epoch{
			random: random,
			hash:   append(append([]byte{}, hash...), 1),
		}))
		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistory_GetEpochs(t *testing.T) {
	h := NewHistory(NewEpoch(nil, 0), NewEpoch(nil, 1))

	require.Len(t, h.GetEpochs(), 2)
}

func TestHistory_GetLast(t *testing.T) {
	h := History{}
	_, ok := h.GetLast()
	require.False(t, ok)

	h = History{epochs: []Epoch{{}, {random: 1}}}
	last, ok := h.GetLast()
	require.True(t, ok)
	require.Equal(t, int64(1), last.random)
}

func TestHistory_Equal(t *testing.T) {
	f := func(hash []byte, random int64) bool {
		h := History{epochs: []Epoch{{hash: hash, random: random}}}

		require.False(t, h.Equal(History{}))
		require.True(t, h.Equal(h))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistory_Serialize(t *testing.T) {
	h := History{}

	data, err := h.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = h.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode history: fake error")
}

func TestHistory_String(t *testing.T) {
	h := History{}
	require.Equal(t, "History[0]{}", h.String())

	h = History{epochs: []Epoch{{}}}
	require.Equal(t, "History[1]{nil}", h.String())

	h = History{epochs: []Epoch{{hash: []byte{0xaa, 0xbb}}}}
	require.Equal(t, "History[1]{aabb}", h.String())
}

func TestHistoryFactory_Deserialize(t *testing.T) {
	factory := HistoryFactory{}

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, History{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode history: fake error")
}

func TestHistories_GetBest(t *testing.T) {
	f := func(r1, r2 int64) bool {
		h1 := History{epochs: []Epoch{{random: r1}}}
		h2 := History{epochs: []Epoch{{random: r2}}}
		hists := Histories{h1, h2}

		if r2 > r1 {
			require.Equal(t, h2, hists.GetBest())
		} else {
			require.Equal(t, h1, hists.GetBest())
		}

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	hists := Histories{}
	best := hists.GetBest()
	require.Equal(t, History{}, best)
}

func TestHistories_Contains(t *testing.T) {
	f := func() bool {
		h1 := makeHistory(3)
		h2 := makeHistory(5)
		h3 := makeHistory(0)
		h4 := makeHistory(1)
		hists := Histories{h1, h2, {}}

		return hists.Contains(h1) && hists.Contains(h2) &&
			!hists.Contains(h3) && !hists.Contains(h4)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestHistories_IsUniqueBest(t *testing.T) {
	f := func(r int64) bool {
		h1 := History{epochs: []Epoch{{random: r}}}
		h2 := History{epochs: []Epoch{{random: r + 1}}}
		h3 := History{epochs: []Epoch{{}, {random: r}}}

		hists := Histories{h1, h2, {}}
		require.True(t, hists.IsUniqueBest(h2))
		require.False(t, hists.IsUniqueBest(h1))

		hists = Histories{h1, h3}
		require.False(t, hists.IsUniqueBest(h1))

		require.False(t, hists.IsUniqueBest(History{}))

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

	hists := NewHistories(ms)
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

func makeHistory(n int) History {
	epochs := make([]Epoch, n)
	for i := range epochs {
		epochs[i] = Epoch{
			random: rand.Int63(),
			hash:   []byte{0xaa},
		}
	}

	h := History{epochs: epochs}

	return h
}
