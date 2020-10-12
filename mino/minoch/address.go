// This file implements the address abstraction for an in-memory implementation.
// Each address uses a unique string to identify the instance it belongs to.
//
// Documentation Last Review: 06.10.2020
//

package minoch

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// Address is the representation of an identifier for minoch.
//
// - implements mino.Address
type address struct {
	orchestrator bool
	id           string
}

// Equal implements mino.Address. It returns true if both addresses are equal.
func (a address) Equal(other mino.Address) bool {
	addr, ok := other.(address)
	return ok && addr.id == a.id
}

// MarshalText implements encoding.TextMarshaler. It returns the string
// representation of an address.
func (a address) MarshalText() ([]byte, error) {
	return []byte(a.id), nil
}

// String implements fmt.Stringer. It returns the address as a string.
func (a address) String() string {
	return a.id
}

// AddressFactory is a factory to deserialize Minoch addresses.
//
// - implements mino.AddressFactory
type AddressFactory struct {
	serde.Factory
}

// FromText implements mino.AddressFactory. It returns an instance of an address
// from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	return address{id: string(text)}
}
