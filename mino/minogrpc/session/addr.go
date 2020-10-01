package session

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

const (
	orchestratorCode = "O"
	followerCode     = "F"
)

// Address is a representation of the network Address of a participant.
//
// - implements mino.Address
type Address struct {
	orchestrator bool
	host         string
}

// NewOrchestratorAddress creates a new address which will be considered as the
// initiator of a protocol.
func NewOrchestratorAddress(addr mino.Address) Address {
	return Address{
		orchestrator: true,
		host:         addr.String(),
	}
}

// NewAddress creates a new address.
func NewAddress(host string) Address {
	return Address{host: host}
}

// GetDialAddress returns a string formatted to be understood by grpc.Dial()
// functions.
func (a Address) GetDialAddress() string {
	return a.host
}

// Equal implements mino.Address. It returns true if both addresses points to
// the same participant.
func (a Address) Equal(other mino.Address) bool {
	switch addr := other.(type) {
	case Address:
		return addr == a
	case wrapAddress:
		return addr.Equal(a)
	}

	return false
}

// MarshalText implements mino.Address. It returns the text format of the
// address that can later be deserialized.
func (a Address) MarshalText() ([]byte, error) {
	data := []byte(followerCode)
	if a.orchestrator {
		data = []byte(orchestratorCode)
	}

	data = append(data, []byte(a.host)...)

	return data, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// address.
func (a Address) String() string {
	return a.host
}

type wrapAddress struct {
	mino.Address
}

func newWrapAddress(addr mino.Address) mino.Address {
	return wrapAddress{
		Address: addr,
	}
}

func (a wrapAddress) Unwrap() mino.Address {
	return a.Address
}

func (a wrapAddress) Equal(other mino.Address) bool {
	me, ok := a.Address.(Address)
	if !ok {
		return a.Address.Equal(other)
	}

	switch addr := other.(type) {
	case Address:
		return addr.host == me.host
	}

	return false
}

// AddressFactory implements mino.AddressFactory
type AddressFactory struct {
	serde.Factory
}

// FromText implements mino.AddressFactory. It returns an instance of an
// address from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	if len(text) == 0 {
		return Address{}
	}

	if string(text)[0] == orchestratorCode[0] {
		return Address{host: string(text)[1:], orchestrator: true}
	}

	return Address{host: string(text)[1:]}
}
