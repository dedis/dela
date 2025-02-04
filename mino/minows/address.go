package minows

import (
	"strings"

	"go.dedis.ch/dela"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const protocolP2P = "/p2p/"

// address represents a publicly reachable network address that can be used
// to establish communication with a remote player through libp2p and,
// therefore, must have both `location` and `identity` components.
// - implements mino.Address
type address struct {
	location ma.Multiaddr
	identity peer.ID
}

// newAddress creates a new address from a publicly reachable location with a
// peer identity.
func newAddress(location ma.Multiaddr, identity peer.ID) (address, error) {
	if location == nil {
		return address{}, xerrors.New("address must have a location")
	}

	err := identity.Validate()
	if err != nil {
		return address{}, xerrors.Errorf("address must have a valid identity: %v", err)
	}

	return address{
		location: location,
		identity: identity,
	}, nil
}

// Equal implements mino.Address.
func (a address) Equal(other mino.Address) bool {
	switch o := other.(type) {
	case address:
		return equalOrBothNil(a.location, o.location) && a.identity == o.identity
	case orchestratorAddr:
		return o.Equal(a)
	}
	return false
}

// String implements fmt.Stringer.
func (a address) String() string {
	var sb strings.Builder

	if a.location != nil {
		sb.WriteString(a.location.String())
	}

	if a.identity.String() != "" {
		sb.WriteString(protocolP2P)
		sb.WriteString(a.identity.String())
	}

	return sb.String()
}

// ConnectionType implements mino.Address
// Not used by minows
func (a address) ConnectionType() mino.AddressConnectionType {
	return mino.ACTws
}

// MarshalText implements encoding.TextMarshaler.
func (a address) MarshalText() ([]byte, error) {
	location := ""
	if a.location != nil {
		location = a.location.String()
	}
	identity := a.identity.String()
	return []byte(strings.Join([]string{location, identity}, ":")), nil
}

// - implements mino.Address
type orchestratorAddr struct {
	address
}

func newOrchestratorAddr(
	location ma.Multiaddr,
	identity peer.ID,
) (orchestratorAddr, error) {
	addr, err := newAddress(location, identity)
	if err != nil {
		return orchestratorAddr{}, err
	}
	return orchestratorAddr{addr}, nil
}

func (a orchestratorAddr) Equal(other mino.Address) bool {
	switch o := other.(type) {
	case orchestratorAddr:
		return equalOrBothNil(a.location, o.location) && a.identity == o.identity
	case address:
		// Allows orchestrator to match its participant public address
		return equalOrBothNil(a.location, o.location)
	}
	return false
}

func (a orchestratorAddr) MarshalText() ([]byte, error) {
	data, _ := a.address.MarshalText()
	return append(data, []byte(":o")...), nil
}

func equalOrBothNil(a, b ma.Multiaddr) bool {
	if a != nil && b != nil {
		return a.Equal(b)
	}
	return a == b
}

// addressFactory is a factory to deserialize Minows addresses.
// - implements mino.AddressFactory
type addressFactory struct {
	serde.Factory
}

// FromText returns an instance of an address
// from a byte slice or nil if anything fails.
// - implements mino.AddressFactory
func (f addressFactory) FromText(text []byte) mino.Address {
	parts := strings.Split(string(text), ":")
	if len(parts) < 2 {
		return nil
	}

	location, err := ma.NewMultiaddr(parts[0])
	if err != nil {
		dela.Logger.Error().Err(err).
			Msgf("could not parse multiaddress %q", parts[0])
		return nil
	}
	identity, err := peer.Decode(parts[1])
	if err != nil {
		dela.Logger.Error().Err(err).
			Msgf("could not decode peer ID %q", parts[1])
		return nil
	}

	var addr mino.Address
	if len(parts) == 2 {
		addr, err = newAddress(location, identity)
	} else if parts[2] == "o" {
		addr, err = newOrchestratorAddr(location, identity)
	}
	if err != nil {
		dela.Logger.Error().Err(err).Msgf("could not create address")
		return nil
	}
	return addr
}
