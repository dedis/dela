// This file contains the implementation of the change set.
//
// Documentation Last Review: 13.10.2020
//

package authority

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var csetFormats = registry.NewSimpleRegistry()

// RegisterChangeSetFormat registers the engine for the provided format.
func RegisterChangeSetFormat(c serde.Format, f serde.FormatEngine) {
	csetFormats.Register(c, f)
}

// RosterChangeSet is the smallest data model to update an authority to another.
//
// - implements authority.ChangeSet
type RosterChangeSet struct {
	remove  []uint
	addrs   []mino.Address
	pubkeys []crypto.PublicKey
}

// NewChangeSet creates a new empty change set.
func NewChangeSet() *RosterChangeSet {
	return &RosterChangeSet{}
}

// GetPublicKeys returns the list of public keys of the new participants.
func (set *RosterChangeSet) GetPublicKeys() []crypto.PublicKey {
	return append([]crypto.PublicKey{}, set.pubkeys...)
}

// GetNewAddresses implements authority.ChangeSet. It returns the list of
// addresses of the new members.
func (set *RosterChangeSet) GetNewAddresses() []mino.Address {
	return append([]mino.Address{}, set.addrs...)
}

// GetRemoveIndices returns the list of indices to remove from the authority.
func (set *RosterChangeSet) GetRemoveIndices() []uint {
	return append([]uint{}, set.remove...)
}

// Remove appends the index to the list of removals.
func (set *RosterChangeSet) Remove(index uint) {
	set.remove = append(set.remove, index)
}

// Add appends the address and the public key to the list of new participants.
func (set *RosterChangeSet) Add(addr mino.Address, pubkey crypto.PublicKey) {
	set.addrs = append(set.addrs, addr)
	set.pubkeys = append(set.pubkeys, pubkey)
}

// NumChanges implements authority.ChangeSet. It returns the number of changes
// that is applied with the change set.
func (set *RosterChangeSet) NumChanges() int {
	return len(set.remove) + len(set.addrs)
}

// Serialize implements serde.Message. It returns the serialized data for this
// change set.
func (set *RosterChangeSet) Serialize(ctx serde.Context) ([]byte, error) {
	format := csetFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, set)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode change set: %v", err)
	}

	return data, nil
}

// PubKeyFac is the key for the public key factory.
type PubKeyFac struct{}

// AddrKeyFac is the key for the address factory.
type AddrKeyFac struct{}

// SimpleChangeSetFactory is a message factory to deserialize a change set.
//
// - roster.ChangeSetFactory
type SimpleChangeSetFactory struct {
	addrFactory   mino.AddressFactory
	pubkeyFactory crypto.PublicKeyFactory
}

// NewChangeSetFactory returns a new change set factory.
func NewChangeSetFactory(af mino.AddressFactory, pkf crypto.PublicKeyFactory) ChangeSetFactory {
	return SimpleChangeSetFactory{
		addrFactory:   af,
		pubkeyFactory: pkf,
	}
}

// Deserialize implements serde.Factory. It returns the change set from the data
// if appropriate, otherwise an error.
func (f SimpleChangeSetFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.ChangeSetOf(ctx, data)
}

// ChangeSetOf implements roster.ChangeSetFactory. It returns the change set
// from the data if appropriate, otherwise an error.
func (f SimpleChangeSetFactory) ChangeSetOf(ctx serde.Context, data []byte) (ChangeSet, error) {
	format := csetFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, PubKeyFac{}, f.pubkeyFactory)
	ctx = serde.WithFactory(ctx, AddrKeyFac{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode change set: %v", err)
	}

	cset, ok := msg.(ChangeSet)
	if !ok {
		return nil, xerrors.Errorf("invalid message of type '%T'", msg)
	}

	return cset, nil
}
