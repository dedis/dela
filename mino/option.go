package mino

import (
	"sort"
)

// Filters is a set of parameters for the Players.Take function.
type Filters struct {
	// Indices indicates the indexes of the elements that must be included. This
	// list if updated based on the filter that we apply. For example, [0,3]
	// tells that this filter keeps 2 elements from the underlying data
	// structure we filter that are stored at indexes 0, 3. This list is always
	// sorted.
	Indices []int
}

// ApplyFilters applies the filters and return the result.
func ApplyFilters(filters []FilterUpdater) *Filters {
	f := &Filters{
		Indices: []int{},
	}

	for _, filter := range filters {
		filter(f)
	}

	return f
}

// FilterUpdater is a function to update the filters.
type FilterUpdater func(*Filters)

// IndexFilter is a filter to include a given index.
func IndexFilter(index int) FilterUpdater {
	return func(filters *Filters) {
		arr := filters.Indices
		i := sort.IntSlice(arr).Search(index)
		// do nothing if the element is already there
		if i < len(arr) && arr[i] == index {
			return
		}

		filters.Indices = append(arr, index)
		sort.Ints(filters.Indices)
	}
}

// RangeFilter is a filter to include a range of indices.
func RangeFilter(start, end int) FilterUpdater {
	return func(filters *Filters) {
		arr := filters.Indices
		queue := []int{}

		i := sort.IntSlice(arr).Search(start)
		for k := start; k < end; k++ {
			if i < len(arr) && arr[i] == k {
				i++
			} else {
				queue = append(queue, k)
			}
		}

		filters.Indices = append(arr, queue...)
		sort.Ints(filters.Indices)
	}
}
