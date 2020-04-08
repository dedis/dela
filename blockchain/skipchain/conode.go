package skipchain

import (
	"io"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Conode is the type of participant for a skipchain. It contains an address
// and a public key that is part of the key pair used to sign blocks.
//
// - implements encoding.Packable
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

// Pack implements encoding.Packable. It returns the protobuf message for the
// conode.
func (c Conode) Pack(encoder encoding.ProtoMarshaler) (proto.Message, error) {
	conode := &ConodeProto{}

	var err error
	conode.Address, err = c.addr.MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal address: %v", err)
	}

	conode.PublicKey, err = encoder.PackAny(c.publicKey)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack public key: %v", err)
	}

	return conode, nil
}

// iterator is a generic implementation of an iterator over a list of conodes.
type iterator struct {
	conodes []Conode
	index   int
}

func (i *iterator) HasNext() bool {
	return i.index+1 < len(i.conodes)
}

func (i *iterator) GetNext() *Conode {
	if i.HasNext() {
		i.index++
		return &i.conodes[i.index]
	}
	return nil
}

// addressIterator is an iterator for a list of addresses.
//
// - implements mino.AddressIterator
type addressIterator struct {
	*iterator
}

// GetNext implements mino.AddressIterator. It returns the next address.
func (i *addressIterator) GetNext() mino.Address {
	conode := i.iterator.GetNext()
	if conode != nil {
		return conode.GetAddress()
	}
	return nil
}

// publicKeyIterator is an iterator for a list of public keys.
//
// - implements crypto.PublicKeyIterator
type publicKeyIterator struct {
	*iterator
}

// GetNext implements crypto.PublicKeyIterator. It returns the next public key.
func (i *publicKeyIterator) GetNext() crypto.PublicKey {
	conode := i.iterator.GetNext()
	if conode != nil {
		return conode.GetPublicKey()
	}
	return nil
}

// Conodes is a list of conodes.
//
// - implements mino.Players
// - implements cosi.CollectiveAuthority
// - implements io.WriterTo
// - implements encoding.Packable
type Conodes []Conode

func newConodes(ca cosi.CollectiveAuthority) Conodes {
	conodes := make([]Conode, ca.Len())
	addrIter := ca.AddressIterator()
	publicKeyIter := ca.PublicKeyIterator()
	for i := range conodes {
		conodes[i] = Conode{
			addr:      addrIter.GetNext(),
			publicKey: publicKeyIter.GetNext(),
		}
	}

	return conodes
}

// Take implements mino.Players. It returns a subset of the conodes.
func (cc Conodes) Take(filters ...mino.FilterUpdater) mino.Players {
	f := mino.ApplyFilters(filters)
	conodes := make(Conodes, len(f.Indices))
	for i, k := range f.Indices {
		conodes[i] = cc[k]
	}
	return conodes
}

// Len implements mino.Players. It returns the length of the list of conodes.
func (cc Conodes) Len() int {
	return len(cc)
}

// GetPublicKey implements cosi.CollectiveAuthority. It returns the public key
// associated with the address and its index.
func (cc Conodes) GetPublicKey(addr mino.Address) (crypto.PublicKey, int) {
	for i, conode := range cc {
		if conode.GetAddress().Equal(addr) {
			return conode.GetPublicKey(), i
		}
	}

	return nil, -1
}

// AddressIterator implements mino.Players. It returns the address iterator.
func (cc Conodes) AddressIterator() mino.AddressIterator {
	return &addressIterator{
		iterator: &iterator{
			index:   -1,
			conodes: cc,
		},
	}
}

// PublicKeyIterator implements cosi.CollectiveAuthority. It returns the public
// key iterator.
func (cc Conodes) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{
		iterator: &iterator{
			index:   -1,
			conodes: cc,
		},
	}
}

// Pack implements encoding.Packable. It converts the list of conodes to a list
// of protobuf messages.
func (cc Conodes) Pack(encoder encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Roster{
		Conodes: make([]*ConodeProto, len(cc)),
	}

	for i, conode := range cc {
		conodepb, err := encoder.Pack(conode)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack conode: %v", err)
		}

		pb.Conodes[i] = conodepb.(*ConodeProto)
	}

	return pb, nil
}

// WriteTo implements io.WriterTo. It writes the roster in the writer.
func (cc Conodes) WriteTo(w io.Writer) (int64, error) {
	sum := int64(0)

	for _, conode := range cc {
		buffer, err := conode.GetPublicKey().MarshalBinary()
		if err != nil {
			return sum, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		n, err := w.Write(buffer)
		sum += int64(n)
		if err != nil {
			return sum, xerrors.Errorf("couldn't write public key: %v", err)
		}

		n, err = w.Write([]byte(conode.GetAddress().String()))
		sum += int64(n)
		if err != nil {
			return sum, xerrors.Errorf("couldn't write address: %v", err)
		}
	}

	return sum, nil
}
