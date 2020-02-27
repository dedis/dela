package blockchain

import (
	"go.dedis.ch/m/crypto"
	mino "go.dedis.ch/m/mino"
)

// SimpleRoster is a list of conodes.
type SimpleRoster struct {
	conodes []*Conode
	pubkeys []crypto.PublicKey
}

// NewRoster returns a new roster made from the list of conodes.
func NewRoster(verifier crypto.Verifier, conodes ...*Conode) (SimpleRoster, error) {
	pubkeys := make([]crypto.PublicKey, len(conodes))
	for i, conode := range conodes {
		pubkey, err := verifier.GetPublicKeyFactory().FromAny(conode.GetPublicKey())
		if err != nil {
			return SimpleRoster{}, err
		}

		pubkeys[i] = pubkey
	}

	return SimpleRoster{
		conodes: conodes,
		pubkeys: pubkeys,
	}, nil
}

// GetConodes returns the list of conodes.
func (r SimpleRoster) GetConodes() []*Conode {
	return r.conodes
}

// GetAddresses returns the list of addresses for the conodes.
func (r SimpleRoster) GetAddresses() []*mino.Address {
	addrs := make([]*mino.Address, len(r.conodes))
	for i, conode := range r.conodes {
		addrs[i] = conode.GetAddress()
	}

	return addrs
}

// GetPublicKeys returns the list of public keys for the conodes.
func (r SimpleRoster) GetPublicKeys() []crypto.PublicKey {
	return r.pubkeys
}
