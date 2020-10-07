// This file implements the address for minogrpc.
//
// Documentation Last Review: 07.10.2020
//

package session

import (
	"fmt"
	"net/url"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const (
	orchestratorCode = "O"
	followerCode     = "F"
)

// Address is a representation of the network Address of a participant. The
// overlay implementation requires a difference between an orchestrator and its
// source address, where the former initiates a protocol and the later
// participates.
//
// See session.wrapAddress for the abstraction provided to a caller external to
// the overlay module.
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

// GetHostname parses the address to extract the hostname.
func (a Address) GetHostname() (string, error) {
	url, err := url.Parse(fmt.Sprintf("//%s", a.host))
	if err != nil {
		return "", xerrors.Errorf("malformed address: %v", err)
	}

	return url.Hostname(), nil
}

// Equal implements mino.Address. It returns true if both addresses are exactly
// similar, in the sense that an orchestrator won't match a follower address
// with the same host.
func (a Address) Equal(other mino.Address) bool {
	switch addr := other.(type) {
	case Address:
		return addr == a
	case wrapAddress:
		// Switch to the wrapped address equality.
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
	if a.orchestrator {
		return fmt.Sprintf("Orchestrator:%s", a.host)
	}

	return a.host
}

// WrapAddress is a super type of the address so that the orchestrator becomes
// equal to its original address. It allows a caller of a protocol to compare
// the actual source address of a request while preserving the orchestrator
// address for the overlay when sending a message back.
//
// - implements mino.Address
type wrapAddress struct {
	mino.Address
}

func newWrapAddress(addr mino.Address) wrapAddress {
	return wrapAddress{
		Address: addr,
	}
}

// Unwrap returns the wrapped address.
func (a wrapAddress) Unwrap() mino.Address {
	return a.Address
}

// Equal implements mino.Address. When it wraps a network address, it will
// consider addresses with the same host as similar, otherwise it returns the
// result of the underlying address comparison. That way, an orchestrator
// address will match the address with the same origin.
func (a wrapAddress) Equal(other mino.Address) bool {
	me, ok := a.Address.(Address)
	if !ok {
		return a.Address.Equal(other)
	}

	switch addr := other.(type) {
	case Address:
		return addr.host == me.host
	case wrapAddress:
		return addr == a
	}

	return false
}

// AddressFactory is a factory for addresses.
//
// - implements mino.AddressFactory
type AddressFactory struct {
	serde.Factory
}

// FromText implements mino.AddressFactory. It returns an instance of an
// address from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	str := string(text)

	if len(str) == 0 {
		return Address{}
	}

	return Address{
		host:         str[1:],
		orchestrator: str[0] == orchestratorCode[0],
	}
}
