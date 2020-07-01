package roster

import (
	"io"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
	"golang.org/x/xerrors"
)

var rosterFormats = registry.NewSimpleRegistry()

func RegisterRoster(c serdeng.Codec, f serdeng.Format) {
	rosterFormats.Register(c, f)
}

// iterator is a generic implementation of an iterator over a list of conodes.
type iterator struct {
	index  int
	roster *Roster
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

// Roster contains a list of participants with their addresses and public keys.
//
// - implements crypto.CollectiveAuthority
// - implements viewchange.Authority
// - implements mino.Players
type Roster struct {
	addrs   []mino.Address
	pubkeys []crypto.PublicKey
}

func New(addrs []mino.Address, pubkeys []crypto.PublicKey) Roster {
	return Roster{
		addrs:   addrs,
		pubkeys: pubkeys,
	}
}

// FromAuthority returns a viewchange roster from a collective authority.
func FromAuthority(authority crypto.CollectiveAuthority) Roster {
	addrs := make([]mino.Address, authority.Len())
	pubkeys := make([]crypto.PublicKey, authority.Len())

	addrIter := authority.AddressIterator()
	pubkeyIter := authority.PublicKeyIterator()
	for i := 0; addrIter.HasNext() && pubkeyIter.HasNext(); i++ {
		addrs[i] = addrIter.GetNext()
		pubkeys[i] = pubkeyIter.GetNext()
	}

	return New(addrs, pubkeys)
}

// Fingerprint implements serde.Fingerprinter. It marshals the roster and writes
// the result in the given writer.
func (r Roster) Fingerprint(w io.Writer) error {
	for i, addr := range r.addrs {
		data, err := addr.MarshalText()
		if err != nil {
			return xerrors.Errorf("couldn't marshal address: %v", err)
		}

		_, err = w.Write(data)
		if err != nil {
			return xerrors.Errorf("couldn't write address: %v", err)
		}

		data, err = r.pubkeys[i].MarshalBinary()
		if err != nil {
			return xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		_, err = w.Write(data)
		if err != nil {
			return xerrors.Errorf("couldn't write public key: %v", err)
		}
	}

	return nil
}

// Take implements mino.Players. It returns a subset of the roster according to
// the filter.
func (r Roster) Take(updaters ...mino.FilterUpdater) mino.Players {
	filter := mino.ApplyFilters(updaters)
	newRoster := Roster{
		addrs:   make([]mino.Address, len(filter.Indices)),
		pubkeys: make([]crypto.PublicKey, len(filter.Indices)),
	}

	for i, k := range filter.Indices {
		newRoster.addrs[i] = r.addrs[k]
		newRoster.pubkeys[i] = r.pubkeys[k]
	}

	return newRoster
}

// Apply implements viewchange.Authority. It returns a new authority after
// applying the change set. The removals must be sorted by descending order and
// unique or the behaviour will be undefined.
func (r Roster) Apply(in viewchange.ChangeSet) viewchange.Authority {
	changeset, ok := in.(ChangeSet)
	if !ok {
		dela.Logger.Warn().Msgf("Change set '%T' is not supported. Ignoring.", in)
		return r
	}

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

	roster := Roster{
		addrs:   addrs,
		pubkeys: pubkeys,
	}

	return roster
}

// Diff implements viewchange.Authority. It returns the change set that must be
// applied to the current authority to get the given one.
func (r Roster) Diff(o viewchange.Authority) viewchange.ChangeSet {
	changeset := ChangeSet{}

	other, ok := o.(Roster)
	if !ok {
		return changeset
	}

	i := 0
	k := 0
	for i < len(r.addrs) || k < len(other.addrs) {
		if i < len(r.addrs) && k < len(other.addrs) {
			if r.addrs[i].Equal(other.addrs[k]) {
				i++
				k++
			} else {
				changeset.Remove = append(changeset.Remove, uint32(i))
				i++
			}
		} else if i < len(r.addrs) {
			changeset.Remove = append(changeset.Remove, uint32(i))
			i++
		} else {
			changeset.Add = append(changeset.Add, Player{
				Address:   other.addrs[k],
				PublicKey: other.pubkeys[k],
			})
			k++
		}
	}

	return changeset
}

// Len implements mino.Players. It returns the length of the roster.
func (r Roster) Len() int {
	return len(r.addrs)
}

// GetPublicKey implements crypto.CollectiveAuthority. It returns the public key
// of the address if it exists, nil otherwise. The second return is the index of
// the public key in the roster.
func (r Roster) GetPublicKey(target mino.Address) (crypto.PublicKey, int) {
	for i, addr := range r.addrs {
		if addr.Equal(target) {
			return r.pubkeys[i], i
		}
	}

	return nil, -1
}

// AddressIterator implements mino.Players. It returns an iterator of the
// addresses of the roster in a deterministic order.
func (r Roster) AddressIterator() mino.AddressIterator {
	return &addressIterator{iterator: &iterator{roster: &r}}
}

// PublicKeyIterator implements crypto.CollectiveAuthority. It returns an
// iterator of the public keys of the roster in a deterministic order.
func (r Roster) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{iterator: &iterator{roster: &r}}
}

// Serialize implements serde.Message.
func (r Roster) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := rosterFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, r)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type RosterFactory struct {
	addrFactory   mino.AddressFactory
	pubkeyFactory crypto.PublicKeyFactory
}

// NewRosterFactory creates a new instance of the authority factory.
func NewRosterFactory(af mino.AddressFactory, pf crypto.PublicKeyFactory) RosterFactory {
	return RosterFactory{
		addrFactory:   af,
		pubkeyFactory: pf,
	}
}

// Deserialize implements serde.Factory.
func (f RosterFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := rosterFormats.Get(ctx.GetName())

	ctx = serdeng.WithFactory(ctx, PubKey{}, f.pubkeyFactory)
	ctx = serdeng.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func (f RosterFactory) AuthorityOf(ctx serdeng.Context, data []byte) (viewchange.Authority, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	roster, ok := msg.(Roster)
	if !ok {
		return nil, xerrors.Errorf("invalid roster")
	}

	return roster, nil
}
