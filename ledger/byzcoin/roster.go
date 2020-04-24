package byzcoin

import (
	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// iterator is a generic implementation of an iterator over a list of conodes.
type iterator struct {
	index  int
	roster *roster
}

func (i *iterator) HasNext() bool {
	return i.index+1 < i.roster.Len()
}

func (i *iterator) GetNext() int {
	if i.HasNext() {
		i.index++
		return i.index
	}
	return -1
}

// addressIterator is an iterator for a list of addresses.
//
// - implements mino.AddressIterator
type addressIterator struct {
	*iterator
}

// GetNext implements mino.AddressIterator. It returns the next address.
func (i *addressIterator) GetNext() mino.Address {
	index := i.iterator.GetNext()
	if index >= 0 {
		return i.roster.addrs[index]
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
	index := i.iterator.GetNext()
	if index >= 0 {
		return i.roster.pubkeys[index]
	}
	return nil
}

type roster struct {
	addrs   []mino.Address
	pubkeys []crypto.PublicKey
}

func (r roster) Take(updaters ...mino.FilterUpdater) mino.Players {
	filter := mino.ApplyFilters(updaters)
	newRoster := roster{
		addrs:   make([]mino.Address, len(filter.Indices)),
		pubkeys: make([]crypto.PublicKey, len(filter.Indices)),
	}

	for i, k := range filter.Indices {
		newRoster.addrs[i] = r.addrs[k]
		newRoster.pubkeys[i] = r.pubkeys[k]
	}

	return newRoster
}

func (r roster) Len() int {
	return len(r.addrs)
}

func (r roster) GetPublicKey(target mino.Address) (crypto.PublicKey, int) {
	for i, addr := range r.addrs {
		if addr.Equal(target) {
			return r.pubkeys[i], i
		}
	}

	return nil, -1
}

func (r roster) AddressIterator() mino.AddressIterator {
	return &addressIterator{iterator: &iterator{roster: &r, index: -1}}
}

func (r roster) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{iterator: &iterator{roster: &r, index: -1}}
}

func (r roster) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Roster{
		Addresses:  make([][]byte, r.Len()),
		PublicKeys: make([]*any.Any, r.Len()),
	}

	var err error
	for i, addr := range r.addrs {
		pb.Addresses[i], err = addr.MarshalText()
		if err != nil {
			return nil, err
		}

		pb.PublicKeys[i], err = enc.PackAny(r.pubkeys[i])
		if err != nil {
			return nil, err
		}
	}

	return pb, nil
}

type rosterFactory struct {
	addressFactory mino.AddressFactory
	pubkeyFactory  crypto.PublicKeyFactory
}

func newRosterFactory(af mino.AddressFactory, pf crypto.PublicKeyFactory) rosterFactory {
	return rosterFactory{
		addressFactory: af,
		pubkeyFactory:  pf,
	}
}

func (f rosterFactory) New(authority crypto.CollectiveAuthority) roster {
	roster := roster{
		addrs:   make([]mino.Address, authority.Len()),
		pubkeys: make([]crypto.PublicKey, authority.Len()),
	}

	addrIter := authority.AddressIterator()
	pubkeyIter := authority.PublicKeyIterator()
	index := 0
	for addrIter.HasNext() && pubkeyIter.HasNext() {
		roster.addrs[index] = addrIter.GetNext()
		roster.pubkeys[index] = pubkeyIter.GetNext()
		index++
	}

	return roster
}

func (f rosterFactory) FromProto(in proto.Message) (crypto.CollectiveAuthority, error) {
	var pb *Roster
	switch msg := in.(type) {
	case *Roster:
		pb = msg
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	if len(pb.Addresses) != len(pb.PublicKeys) {
		return nil, xerrors.Errorf("mismatch array length")
	}

	roster := roster{
		addrs:   make([]mino.Address, len(pb.Addresses)),
		pubkeys: make([]crypto.PublicKey, len(pb.PublicKeys)),
	}

	var err error
	for i, addrpb := range pb.GetAddresses() {
		roster.addrs[i] = f.addressFactory.FromText(addrpb)

		roster.pubkeys[i], err = f.pubkeyFactory.FromProto(pb.GetPublicKeys()[i])
		if err != nil {
			return nil, err
		}
	}

	return roster, nil
}
