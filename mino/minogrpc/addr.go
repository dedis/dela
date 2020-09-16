package minogrpc

import (
	"bytes"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// rootAddress is the address of the orchestrator of a protocol. When Stream is
// called, the caller takes this address so that participants know how to route
// message to it.
//
// - implements mino.Address
type rootAddress struct{}

func newRootAddress() rootAddress {
	return rootAddress{}
}

// Equal implements mino.Address. It returns true if the other address is also a
// root address.
func (a rootAddress) Equal(other mino.Address) bool {
	addr, ok := other.(rootAddress)
	return ok && a == addr
}

// MarshalText implements mino.Address. It returns a buffer that uses the
// private area of Unicode to define the root.
func (a rootAddress) MarshalText() ([]byte, error) {
	return orchestratorBytes, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// address.
func (a rootAddress) String() string {
	return orchestratorDescription
}

// address is a representation of the network address of a participant.
//
// - implements mino.Address
type address struct {
	host string
}

// GetDialAddress returns a string formatted to be understood by grpc.Dial()
// functions.
func (a address) GetDialAddress() string {
	return a.host
}

// Equal implements mino.Address. It returns true if both addresses points to
// the same participant.
func (a address) Equal(other mino.Address) bool {
	addr, ok := other.(address)
	return ok && addr == a
}

// MarshalText implements mino.Address. It returns the text format of the
// address that can later be deserialized.
func (a address) MarshalText() ([]byte, error) {
	return []byte(a.host), nil
}

// String implements fmt.Stringer. It returns a string representation of the
// address.
func (a address) String() string {
	return a.host
}

// AddressFactory implements mino.AddressFactory
type AddressFactory struct {
	serde.Factory
}

// FromText implements mino.AddressFactory. It returns an instance of an
// address from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	if bytes.Equal(text, orchestratorBytes) {
		return newRootAddress()
	}

	return address{host: string(text)}
}
