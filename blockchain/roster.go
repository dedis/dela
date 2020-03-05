package blockchain

import (
	"io"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/crypto"
	mino "go.dedis.ch/fabric/mino"
)

// SimpleRoster is a list of conodes.
type SimpleRoster struct {
	addrs   []*mino.Address
	pubkeys []crypto.PublicKey
}

// NewRoster returns a new roster made from the list of conodes.
func NewRoster(verifier crypto.Verifier, conodes ...*Conode) (SimpleRoster, error) {
	pubkeys := make([]crypto.PublicKey, len(conodes))
	addrs := make([]*mino.Address, len(conodes))
	for i, conode := range conodes {
		pubkey, err := verifier.GetPublicKeyFactory().FromProto(conode.PublicKey)
		if err != nil {
			return SimpleRoster{}, err
		}

		pubkeys[i] = pubkey
		addrs[i] = conode.Address
	}

	return SimpleRoster{
		addrs:   addrs,
		pubkeys: pubkeys,
	}, nil
}

// GetAddresses returns the list of addresses for the conodes.
func (r SimpleRoster) GetAddresses() []*mino.Address {
	return r.addrs
}

// GetPublicKeys returns the list of public keys for the conodes.
func (r SimpleRoster) GetPublicKeys() []crypto.PublicKey {
	return r.pubkeys
}

func (r SimpleRoster) GetConodes() ([]*Conode, error) {
	conodes := make([]*Conode, len(r.addrs))
	for i, addr := range r.addrs {
		packed, err := r.pubkeys[i].Pack()
		if err != nil {
			return nil, err
		}

		any, err := ptypes.MarshalAny(packed)
		if err != nil {
			return nil, err
		}

		conodes[i] = &Conode{
			Address:   addr,
			PublicKey: any,
		}
	}

	return conodes, nil
}

// WriteTo casts the roster into bytes and write them to the writer.
func (r SimpleRoster) WriteTo(w io.Writer) (int64, error) {
	sum := int64(0)
	for i, pubkey := range r.pubkeys {
		sigbuf, err := pubkey.MarshalBinary()
		if err != nil {
			return sum, err
		}

		n, err := w.Write(sigbuf)
		sum += int64(n)
		if err != nil {
			return sum, err
		}

		buffer, err := proto.Marshal(r.addrs[i])
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
