package roster

import (
	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// iterator is a generic implementation of an iterator over a list of conodes.
type iterator struct {
	index  int
	roster *roster
}

func (i *iterator) Seek(index int) {
	i.index = index
}

func (i *iterator) HasNext() bool {
	return i.index < i.roster.Len()
}

func (i *iterator) GetNext() int {
	res := i.index
	i.index++
	return res
}

// addressIterator is an iterator for a list of addresses.
//
// - implements mino.AddressIterator
type addressIterator struct {
	*iterator
}

// GetNext implements mino.AddressIterator. It returns the next address.
func (i *addressIterator) GetNext() mino.Address {
	if i.iterator.HasNext() {
		return i.roster.addrs[i.iterator.GetNext()]
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
	if i.iterator.HasNext() {
		return i.roster.pubkeys[i.iterator.GetNext()]
	}
	return nil
}

// roster contains a list of participants with their addresses and public keys.
//
// - implements crypto.CollectiveAuthority
// - implements viewchange.EvolvableAuthority
// - implements mino.Players
// - implements encoding.Packable
type roster struct {
	leader  int
	addrs   []mino.Address
	pubkeys []crypto.PublicKey
}

// New returns a viewchange roster from a collective authority.
func New(authority crypto.CollectiveAuthority) viewchange.Authority {
	addrs := make([]mino.Address, authority.Len())
	pubkeys := make([]crypto.PublicKey, authority.Len())

	addrIter := authority.AddressIterator()
	pubkeyIter := authority.PublicKeyIterator()
	for i := 0; addrIter.HasNext() && pubkeyIter.HasNext(); i++ {
		addrs[i] = addrIter.GetNext()
		pubkeys[i] = pubkeyIter.GetNext()
	}

	roster := roster{
		addrs:   addrs,
		pubkeys: pubkeys,
	}

	return roster
}

// Take implements mino.Players. It returns a subset of the roster according to
// the filter.
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

// Apply implements viewchange.EvolvableAuthority. It returns a new authority
// after applying the change set. The removals must be sorted by descending
// order and unique or the behaviour will be undefined.
func (r roster) Apply(changeset viewchange.ChangeSet) viewchange.Authority {
	addrs := make([]mino.Address, r.Len())
	pubkeys := make([]crypto.PublicKey, r.Len())

	for i, addr := range r.addrs {
		addrs[i] = addr
		pubkeys[i] = r.pubkeys[i]
	}

	for _, i := range changeset.Remove {
		if int(i) < len(addrs) {
			addrs = append(addrs[:i], addrs[i+1:]...)
			pubkeys = append(pubkeys[:i], pubkeys[i+1:]...)
		}
	}

	for _, player := range changeset.Add {
		addrs = append(addrs, player.Address)
		pubkeys = append(pubkeys, player.PublicKey)
	}

	roster := roster{
		addrs:   addrs,
		pubkeys: pubkeys,
	}

	return roster
}

func (r roster) Diff(viewchange.Authority) viewchange.ChangeSet {
	return viewchange.ChangeSet{}
}

// Len implements mino.Players. It returns the length of the roster.
func (r roster) Len() int {
	return len(r.addrs)
}

// GetPublicKey implements crypto.CollectiveAuthority. It returns the public key
// of the address if it exists, nil otherwise. The second return is the index of
// the public key in the roster.
func (r roster) GetPublicKey(target mino.Address) (crypto.PublicKey, int) {
	for i, addr := range r.addrs {
		if addr.Equal(target) {
			return r.pubkeys[i], i
		}
	}

	return nil, -1
}

// AddressIterator implements mino.Players. It returns an iterator of the
// addresses of the roster in a deterministic order.
func (r roster) AddressIterator() mino.AddressIterator {
	return &addressIterator{iterator: &iterator{roster: &r}}
}

// PublicKeyIterator implements crypto.CollectiveAuthority. It returns an
// iterator of the public keys of the roster in a deterministic order.
func (r roster) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{iterator: &iterator{roster: &r}}
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// roster.
func (r roster) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	addrs := make([][]byte, r.Len())
	pubkeys := make([]*any.Any, r.Len())

	var err error
	for i, addr := range r.addrs {
		addrs[i], err = addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		pubkeys[i], err = enc.PackAny(r.pubkeys[i])
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack public key: %v", err)
		}
	}

	pb := &Roster{
		Addresses:  addrs,
		PublicKeys: pubkeys,
	}

	return pb, nil
}

// Factory provide functions to create and decode a roster.
type Factory interface {
	GetAddressFactory() mino.AddressFactory
	GetPublicKeyFactory() crypto.PublicKeyFactory
	FromProto(proto.Message) (viewchange.Authority, error)
}

type defaultFactory struct {
	addressFactory mino.AddressFactory
	pubkeyFactory  crypto.PublicKeyFactory
}

// NewRosterFactory creates a new instance of the authority factory.
func NewRosterFactory(af mino.AddressFactory, pf crypto.PublicKeyFactory) Factory {
	return defaultFactory{
		addressFactory: af,
		pubkeyFactory:  pf,
	}
}

// GetAddressFactory implements viewchange.AuthorityFactory. It returns the
// address factory.
func (f defaultFactory) GetAddressFactory() mino.AddressFactory {
	return f.addressFactory
}

// GetPublicKeyFactory implements viewchange.AuthorityFactory. It returns the
// public key factory.
func (f defaultFactory) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return f.pubkeyFactory
}

// FromProto implements viewchange.AuthorityFactory. It returns the roster
// associated with the message if appropriate, otherwise an error.
func (f defaultFactory) FromProto(in proto.Message) (viewchange.Authority, error) {
	var pb *Roster
	switch msg := in.(type) {
	case *Roster:
		pb = msg
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	if len(pb.Addresses) != len(pb.PublicKeys) {
		return nil, xerrors.Errorf("mismatch array length %d != %d",
			len(pb.Addresses), len(pb.PublicKeys))
	}

	addrs := make([]mino.Address, len(pb.Addresses))
	pubkeys := make([]crypto.PublicKey, len(pb.PublicKeys))

	for i, addrpb := range pb.GetAddresses() {
		addrs[i] = f.addressFactory.FromText(addrpb)

		pubkey, err := f.pubkeyFactory.FromProto(pb.GetPublicKeys()[i])
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode public key: %v", err)
		}

		pubkeys[i] = pubkey
	}

	roster := roster{
		addrs:   addrs,
		pubkeys: pubkeys,
	}

	return roster, nil
}
