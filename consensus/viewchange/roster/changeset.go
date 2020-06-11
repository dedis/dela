package roster

import (
	"go.dedis.ch/dela/consensus/viewchange/roster/json"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// Player is a container for an address and a public key.
type Player struct {
	Address   mino.Address
	PublicKey crypto.PublicKey
}

// ChangeSet is the smallest data model to update an authority to another.
type ChangeSet struct {
	serde.UnimplementedMessage

	Remove []uint32
	Add    []Player
}

// VisitJSON implements serde.Message.
func (set ChangeSet) VisitJSON(ser serde.Serializer) (interface{}, error) {
	add := make([]json.Player, len(set.Add))
	for i, player := range set.Add {
		addr, err := player.Address.MarshalText()
		if err != nil {
			return nil, err
		}

		pubkey, err := ser.Serialize(player.PublicKey)
		if err != nil {
			return nil, err
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

type ChangeSetFactory struct {
	serde.UnimplementedFactory

	addrFactory   mino.AddressFactory
	pubkeyFactory serde.Factory
}

func NewChangeSetFactory(af mino.AddressFactory, kf serde.Factory) ChangeSetFactory {
	return ChangeSetFactory{
		addrFactory:   af,
		pubkeyFactory: kf,
	}
}

func (f ChangeSetFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.ChangeSet{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	add := make([]Player, len(m.Add))
	for i, player := range m.Add {
		addr := f.addrFactory.FromText(player.Address)

		var pubkey crypto.PublicKey
		err = in.GetSerializer().Deserialize(player.PublicKey, f.pubkeyFactory, &pubkey)
		if err != nil {
			return nil, err
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
