package mino

// addressIterator is an implementation of the iterator for addresses.
//
// - implements mino.AddressIterator
type addressIterator struct {
	index int
	addrs []Address
}

// Seek implements mino.AddressIterator.
func (it *addressIterator) Seek(index int) {
	it.index = index
}

// HasNext implements mino.AddressIterator. It returns true if there is an
// address available.
func (it *addressIterator) HasNext() bool {
	return it.index < len(it.addrs)
}

// GetNext implements mino.AddressIterator. It returns the address at the
// current index and moves the iterator to the next address.
func (it *addressIterator) GetNext() Address {
	if it.HasNext() {
		res := it.addrs[it.index]
		it.index++
		return res
	}
	return nil
}

// roster is an implementation of the mino.Players interface. It provides helper
// when known addresses need to be grouped into a roster for Mino calls.
//
// - implements mino.Players
type roster struct {
	addrs []Address
}

// NewAddresses is a helper to instantiate a Players interface with only a few
// addresses.
func NewAddresses(addrs ...Address) Players {
	return roster{addrs: addrs}
}

// Take implements mino.Players. It returns a subset of the roster according to
// the filter.
func (r roster) Take(updaters ...FilterUpdater) Players {
	filter := &Filter{}
	for _, fn := range updaters {
		fn(filter)
	}

	addrs := make([]Address, len(filter.Indices))
	for i, k := range filter.Indices {
		addrs[i] = r.addrs[k]
	}

	newRoster := roster{addrs: addrs}

	return newRoster
}

// AddressIterator implements mino.Players. It returns an iterator for the
// roster.
func (r roster) AddressIterator() AddressIterator {
	return &addressIterator{addrs: r.addrs}
}

// Len implements mino.Players. It returns the length of the roster.
func (r roster) Len() int {
	return len(r.addrs)
}
