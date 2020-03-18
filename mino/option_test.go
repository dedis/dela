package mino

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilters_ParseFilters(t *testing.T) {
	filters := ApplyFilters([]FilterUpdater{IndexFilter(1)})
	require.Equal(t, []int{1}, filters.Indices)
}

func TestFilters_IndexFilter(t *testing.T) {
	filters := &Filters{Indices: []int{}}

	IndexFilter(1)(filters)
	require.Equal(t, filters.Indices, []int{1})

	IndexFilter(2)(filters)
	require.Equal(t, filters.Indices, []int{1, 2})

	IndexFilter(0)(filters)
	require.Equal(t, filters.Indices, []int{0, 1, 2})

	IndexFilter(0)(filters)
	IndexFilter(1)(filters)
	IndexFilter(2)(filters)
	require.Equal(t, filters.Indices, []int{0, 1, 2})
}

func TestFilters_RangeFilter(t *testing.T) {
	filters := &Filters{Indices: []int{}}

	RangeFilter(2, 5)(filters)
	require.Equal(t, []int{2, 3, 4}, filters.Indices)

	RangeFilter(0, 3)(filters)
	require.Equal(t, []int{0, 1, 2, 3, 4}, filters.Indices)

	RangeFilter(3, 7)(filters)
	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6}, filters.Indices)

	RangeFilter(2, 5)(filters)
	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6}, filters.Indices)

	filters = &Filters{Indices: []int{0, 1, 4, 5}}
	RangeFilter(1, 7)(filters)
	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6}, filters.Indices)
}
