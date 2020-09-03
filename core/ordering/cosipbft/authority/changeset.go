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

// Player is a container for an address and a public key.
type Player struct {
	Address   mino.Address
	PublicKey crypto.PublicKey
}

// RosterChangeSet is the smallest data model to update an authority to another.
//
// - implements serde.Message
type RosterChangeSet struct {
	Remove []uint32
	Add    []Player
}

// NumChanges implements viewchange.ChangeSet. It returns the number of changes
// that is applied with the change set.
func (set RosterChangeSet) NumChanges() int {
	return len(set.Remove) + len(set.Add)
}

// GetNewAddresses implements viewchange.ChangeSet. It returns the list of
// addresses of the new members.
func (set RosterChangeSet) GetNewAddresses() []mino.Address {
	addrs := make([]mino.Address, len(set.Add))
	for i, player := range set.Add {
		addrs[i] = player.Address
	}

	return addrs
}

// Serialize implements serde.Message.
func (set RosterChangeSet) Serialize(ctx serde.Context) ([]byte, error) {
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
// - viewchange.ChangeSetFactory
// - implements serde.Factory
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

// ChangeSetOf implements viewchange.ChangeSetFactory. It returns the change set
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
