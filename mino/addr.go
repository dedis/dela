//
// Documentation Last Review: 07.10.2020
//

package mino

// NewAddressIterator returns a new address iterator.
func NewAddressIterator(addrs []Address) AddressIterator {
	return &addressIterator{
		addrs: addrs,
	}
}

// addressIterator is an implementation of the iterator for addresses.
//
// - implements mino.AddressIterator
type addressIterator struct {
	index int
	addrs []Address
}

// Seek implements mino.AddressIterator. It moves the iterator to a specific
// position. The next address can be either defined or nil, it is the
// responsibility of the caller to verify.
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
	res := it.addrs[it.index]
	it.index++
	return res
}

// roster is an implementation of the players interface. It provides helper when
// known addresses need to be grouped into a roster for Mino calls.
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
// authority.
func (r roster) AddressIterator() AddressIterator {
	return NewAddressIterator(r.addrs)
}

// Len implements mino.Players. It returns the length of the authority.
func (r roster) Len() int {
	return len(r.addrs)
}
