package blockchain

import (
	"io"

	proto "github.com/golang/protobuf/proto"
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

// WriteTo casts the roster into bytes and write them to the writer.
func (r SimpleRoster) WriteTo(w io.Writer) (int64, error) {
	sum := int64(0)
	for i, conode := range r.conodes {
		sigbuf, err := r.pubkeys[i].MarshalBinary()
		if err != nil {
			return sum, err
		}

		n, err := w.Write(sigbuf)
		sum += int64(n)
		if err != nil {
			return sum, err
		}

		buffer, err := proto.Marshal(conode.GetAddress())
		if err != nil {
			return sum, err
		}

		n, err = w.Write(buffer)
		sum += int64(n)
		if err != nil {
			return sum, err
		}
	}

	return sum, nil
}
