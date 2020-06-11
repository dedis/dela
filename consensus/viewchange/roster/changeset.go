package roster

import (
	"go.dedis.ch/dela/consensus/viewchange/roster/json"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Player is a container for an address and a public key.
type Player struct {
	Address   mino.Address
	PublicKey crypto.PublicKey
}

// ChangeSet is the smallest data model to update an authority to another.
//
// - implements serde.Message
type ChangeSet struct {
	serde.UnimplementedMessage

	Remove []uint32
	Add    []Player
}

// VisitJSON implements serde.Message. It serializes the change set in a JSON
// message.
func (set ChangeSet) VisitJSON(ser serde.Serializer) (interface{}, error) {
	add := make([]json.Player, len(set.Add))
	for i, player := range set.Add {
		addr, err := player.Address.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize address: %v", err)
		}

		pubkey, err := ser.Serialize(player.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize public key: %v", err)
		}

		add[i] = json.Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}

	m := json.ChangeSet{
		Remove: set.Remove,
		Add:    add,
	}

	return m, nil
}

// ChangeSetFactory is a message factory to deserialize a change set.
//
// - implements serde.Factory
type ChangeSetFactory struct {
	serde.UnimplementedFactory

	addrFactory   mino.AddressFactory
	pubkeyFactory serde.Factory
}

// NewChangeSetFactory returns a new change set factory.
func NewChangeSetFactory(af mino.AddressFactory, kf serde.Factory) ChangeSetFactory {
	return ChangeSetFactory{
		addrFactory:   af,
		pubkeyFactory: kf,
	}
}

// VisitJSON implements serde.Factory. It deserializes the change set in JSON
// format.
func (f ChangeSetFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.ChangeSet{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize change set: %v", err)
	}

	add := make([]Player, len(m.Add))
	for i, player := range m.Add {
		addr := f.addrFactory.FromText(player.Address)

		var pubkey crypto.PublicKey
		err = in.GetSerializer().Deserialize(player.PublicKey, f.pubkeyFactory, &pubkey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize public key: %v", err)
		}

		add[i] = Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}

	set := ChangeSet{
		Remove: m.Remove,
		Add:    add,
	}

	return set, nil
}
