//
// Documentation Last Review: 07.10.2020
//

package mino

import (
	"sort"
)

// Filter is a set of parameters for the Players.Take function.
type Filter struct {
	// Indices indicates the indexes of the elements that must be included. This
	// list if updated based on the filter that we apply. For example, [0,3]
	// tells that this filter keeps 2 elements from the underlying data
	// structure we filter that are stored at indexes 0, 3. This list is always
	// sorted and can be shifted in a circular way.
	Indices []int
}

// ApplyFilters applies the filters and return the result.
func ApplyFilters(filters []FilterUpdater) *Filter {
	f := &Filter{
		Indices: []int{},
	}

	for _, filter := range filters {
		filter(f)
	}

	return f
}

// FilterUpdater is a function to update the filters.
type FilterUpdater func(*Filter)

// RotateFilter is a filter to rotate the indices. When n is above zero, it will
// rotate by n steps on the left and when n is below, it will do the same on the
// right. The behaviour is unknown if not used as the last filter as next
// updaters could change the order.
func RotateFilter(n int) FilterUpdater {
	return func(filter *Filter) {
		if len(filter.Indices) == 0 {
			return
		}

		n = n % len(filter.Indices)
		if n < 0 {
			n += len(filter.Indices)
		}

		filter.Indices = append(filter.Indices[n:], filter.Indices[:n]...)
	}
}

// IndexFilter is a filter to include a given index.
func IndexFilter(index int) FilterUpdater {
	return func(filters *Filter) {
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
	return func(filters *Filter) {
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

// ListFilter is a filter to set the list of indices. It will override any index
// previously set.
func ListFilter(indices []int) FilterUpdater {
	return func(filters *Filter) {
		filters.Indices = indices
	}
}
