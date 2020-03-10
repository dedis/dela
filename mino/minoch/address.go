package minoch

import "go.dedis.ch/fabric/mino"

// Address is the representation of an identifier for minoch.
type address struct {
	id string
}

// MarshalText returns the string representation of an address.
func (a address) MarshalText() ([]byte, error) {
	return []byte(a.id), nil
}

// String returns the address as a string.
func (a address) String() string {
	return a.id
}

// AddressFactory is an implementation of the factory interface.
type AddressFactory struct{}

// FromText returns an instance of an address from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	return address{id: string(text)}
}
