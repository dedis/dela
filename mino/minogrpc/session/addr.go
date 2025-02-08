// This file implements the address for minogrpc.
//
// Documentation Last Review: 07.10.2020
//

package session

import (
	"fmt"
	"net/url"
	"strings"

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
// source address, where the former initiates a protocol and the latter
// participates.
//
// See session.wrapAddress for the abstraction provided to a caller external to
// the overlay module.
//
// - implements mino.Address
type Address struct {
	orchestrator   bool
	host           string
	connectionType mino.AddressConnectionType
}

// NewOrchestratorAddress creates a new address which will be considered as the
// initiator of a protocol.
func NewOrchestratorAddress(addr mino.Address) Address {
	a := NewAddress(addr.String())
	a.orchestrator = true
	return a
}

// NewAddress creates a new address.
func NewAddress(host string) Address {
	hostSlash := host
	if !strings.Contains(host, "//") {
		hostSlash = "//" + host
	}
	u, err := url.Parse(hostSlash)
	if err != nil {
		return Address{connectionType: mino.ACTgRPCS, host: host}
	}
	a, err := NewAddressFromURL(*u)
	if err != nil {
		return Address{connectionType: mino.ACTgRPCS, host: host}
	}
	return a
}

// NewAddressFromURL creates a new address given a URL.
func NewAddressFromURL(addr url.URL) (Address, error) {
	var a Address
	a.host = addr.Host

	if addr.Port() == "" {
		return a, xerrors.Errorf("no port given or not able to infer it from protocol")
	}

	scheme := addr.Scheme
	// This seems to be the default when looking at tests.
	if scheme == "" {
		scheme = "grpcs"
	}

	switch scheme {
	case "grpc":
		a.connectionType = mino.ACTgRPC
	case "grpcs":
		a.connectionType = mino.ACTgRPCS
	case "https":
		a.connectionType = mino.ACThttps
	default:
		return a, xerrors.Errorf("unknown scheme '%s' in address", addr.Scheme)
	}

	a.host = addr.Host

	return a, nil
}

// GetDialAddress returns a string formatted to be understood by grpc.Dial()
// functions.
func (a Address) GetDialAddress() string {
	return a.host
}

// ConnectionType returns how to connect to the other host
func (a Address) ConnectionType() mino.AddressConnectionType {
	return a.connectionType
}

// GetHostname parses the address to extract the hostname.
func (a Address) GetHostname() (string, error) {
	url, err := url.Parse(fmt.Sprintf("//%s", a.host))
	if err != nil {
		return "", xerrors.Errorf("malformed address: %v", err)
	}

	return url.Hostname(), nil
}

// Equal implements 'mino.Address'. It returns true if both addresses
// are exactly similar, in the sense that an orchestrator won't match
// a follower address with the same host.
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
	data = append(data, byte(a.connectionType+'A'))
	data = append(data, []byte(a.host)...)

	return data, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// address.
func (a Address) String() string {
	url := "grpcs://"
	switch a.connectionType {
	case mino.ACTgRPCS:
		url += a.host
	case mino.ACTgRPC:
		url = "grpc://" + a.host
	case mino.ACThttps:
		url = "https://" + a.host
	case mino.ACTws:
		url = "ws://" + a.host
	default:
		url = "unknown://" + a.host
	}
	if a.orchestrator {
		return "Orchestrator:" + url
	}

	return url
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

// Equal implements 'mino.Address'. When it wraps a network address, it will
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
func (f AddressFactory) FromText(buf []byte) mino.Address {
	str := string(buf)

	if len(str) < 2 {
		return Address{}
	}

	return Address{
		orchestrator:   str[0] == orchestratorCode[0],
		connectionType: mino.AddressConnectionType(buf[1] - 'A'),
		host:           str[2:],
	}
}
