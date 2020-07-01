package roster

import (
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var csetFormats = registry.NewSimpleRegistry()

func RegisterChangeSet(c serde.Codec, f serde.Format) {
	csetFormats.Register(c, f)
}

// Player is a container for an address and a public key.
type Player struct {
	Address   mino.Address
	PublicKey crypto.PublicKey
}

// ChangeSet is the smallest data model to update an authority to another.
//
// - implements serde.Message
type ChangeSet struct {
	Remove []uint32
	Add    []Player
}

// Serialize implements serde.Message.
func (set ChangeSet) Serialize(ctx serde.Context) ([]byte, error) {
	format := csetFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, set)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type PubKey struct{}
type AddrKey struct{}

// ChangeSetFactory is a message factory to deserialize a change set.
//
// - implements serde.Factory
type ChangeSetFactory struct {
	addrFactory   mino.AddressFactory
	pubkeyFactory crypto.PublicKeyFactory
}

// NewChangeSetFactory returns a new change set factory.
func NewChangeSetFactory(af mino.AddressFactory, pkf crypto.PublicKeyFactory) ChangeSetFactory {
	return ChangeSetFactory{
		addrFactory:   af,
		pubkeyFactory: pkf,
	}
}

// Deserialize implements serde.Factory.
func (f ChangeSetFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := csetFormats.Get(ctx.GetName())

	ctx = serde.WithFactory(ctx, PubKey{}, f.pubkeyFactory)
	ctx = serde.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func (f ChangeSetFactory) ChangeSetOf(ctx serde.Context, data []byte) (viewchange.ChangeSet, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	cset, ok := msg.(ChangeSet)
	if !ok {
		return nil, xerrors.New("invalid change set")
	}

	return cset, nil
}
