package skipchain

import (
	"io"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// Conode is the type of participant for a skipchain. It contains an address
// and a public key that is part of the key pair used to sign blocks.
type Conode struct {
	addr      *mino.Address
	publicKey crypto.PublicKey
}

// GetAddress returns the address of the conode.
func (c Conode) GetAddress() *mino.Address {
	return c.addr
}

// GetPublicKey returns the public key of the conode.
func (c Conode) GetPublicKey() crypto.PublicKey {
	return c.publicKey
}

// Pack returns the protobuf message for the conode.
func (c Conode) Pack() (proto.Message, error) {
	packed, err := c.publicKey.Pack()
	if err != nil {
		return nil, err
	}

	conode := &blockchain.Conode{Address: c.addr}
	conode.PublicKey, err = ptypes.MarshalAny(packed)
	if err != nil {
		return nil, err
	}

	return conode, nil
}

// Conodes is a list of conodes.
type Conodes []Conode

// NewConodesFromProto returns the list of conodes from the protobuf messages.
func NewConodesFromProto(v crypto.Verifier, msgs []*blockchain.Conode) (Conodes, error) {
	conodes := make(Conodes, len(msgs))
	for i, msg := range msgs {
		publicKey, err := v.GetPublicKeyFactory().FromProto(msg.GetPublicKey())
		if err != nil {
			return nil, err
		}

		conodes[i] = Conode{
			addr:      msg.GetAddress(),
			publicKey: publicKey,
		}
	}
	return conodes, nil
}

// GetAddresses returns the list of addresses.
func (cc Conodes) GetAddresses() []*mino.Address {
	addrs := make([]*mino.Address, len(cc))
	for i, conode := range cc {
		addrs[i] = conode.GetAddress()
	}
	return addrs
}

// GetPublicKeys returns the list of public keys.
func (cc Conodes) GetPublicKeys() []crypto.PublicKey {
	pubkeys := make([]crypto.PublicKey, len(cc))
	for i, conode := range cc {
		pubkeys[i] = conode.GetPublicKey()
	}
	return pubkeys
}

// ToProto converts the list of conodes to a list of protobuf messages.
func (cc Conodes) ToProto() ([]*blockchain.Conode, error) {
	conodes := make([]*blockchain.Conode, len(cc))
	for i, conode := range cc {
		packed, err := conode.Pack()
		if err != nil {
			return nil, err
		}

		conodes[i] = packed.(*blockchain.Conode)
	}

	return conodes, nil
}

// WriteTo write the conode's bytes in the writer.
func (cc Conodes) WriteTo(w io.Writer) (int64, error) {
	sum := int64(0)

	for _, conode := range cc {
		buffer, err := conode.GetPublicKey().MarshalBinary()
		if err != nil {
			return sum, err
		}

		n, err := w.Write(buffer)
		sum += int64(n)
		if err != nil {
			return sum, err
		}

		buffer, err = proto.Marshal(conode.GetAddress())
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
