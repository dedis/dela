package skipchain

import (
	"io"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// Conode is the type of participant for a skipchain. It contains an address
// and a public key that is part of the key pair used to sign blocks.
type Conode struct {
	addr      mino.Address
	publicKey crypto.PublicKey
}

// GetAddress returns the address of the conode.
func (c Conode) GetAddress() mino.Address {
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

	conode := &ConodeProto{}

	conode.Address, err = c.addr.MarshalText()
	if err != nil {
		return nil, err
	}

	conode.PublicKey, err = ptypes.MarshalAny(packed)
	if err != nil {
		return nil, err
	}

	return conode, nil
}

type iterator struct {
	conodes []Conode
	index   int
}

func (i *iterator) HasNext() bool {
	if i.index+1 < len(i.conodes) {
		return true
	}
	return false
}

func (i *iterator) GetNext() *Conode {
	if i.HasNext() {
		i.index++
		return &i.conodes[i.index]
	}
	return nil
}

type addressIterator struct {
	*iterator
}

func (i *addressIterator) GetNext() mino.Address {
	conode := i.iterator.GetNext()
	if conode != nil {
		return conode.GetAddress()
	}
	return nil
}

type publicKeyIterator struct {
	*iterator
}

func (i *publicKeyIterator) GetNext() crypto.PublicKey {
	conode := i.iterator.GetNext()
	if conode != nil {
		return conode.GetPublicKey()
	}
	return nil
}

// Conodes is a list of conodes.
type Conodes []Conode

// Take returns a subset of the conodes.
func (cc Conodes) Take(filters ...mino.Filter) mino.Players {
	f := mino.ParseFilters(filters)
	conodes := make(Conodes, len(f.Indices))
	for i, k := range f.Indices {
		conodes[i] = cc[k]
	}

	return conodes
}

// Len returns the length of the list of conodes.
func (cc Conodes) Len() int {
	return len(cc)
}

// AddressIterator returns the address iterator.
func (cc Conodes) AddressIterator() mino.AddressIterator {
	return &addressIterator{
		iterator: &iterator{
			index:   -1,
			conodes: cc,
		},
	}
}

// PublicKeyIterator returns the public key iterator.
func (cc Conodes) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{
		iterator: &iterator{
			index:   -1,
			conodes: cc,
		},
	}
}

// ToProto converts the list of conodes to a list of protobuf messages.
func (cc Conodes) ToProto() ([]*ConodeProto, error) {
	conodes := make([]*ConodeProto, len(cc))
	for i, conode := range cc {
		packed, err := conode.Pack()
		if err != nil {
			return nil, err
		}

		conodes[i] = packed.(*ConodeProto)
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

		n, err = w.Write([]byte(conode.GetAddress().String()))
		sum += int64(n)
		if err != nil {
			return sum, err
		}
	}

	return sum, nil
}
