package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	authority.RegisterChangeSetFormat(serde.FormatJSON, changeSetFormat{})
	authority.RegisterRosterFormat(serde.FormatJSON, rosterFormat{})
}

// Player is a JSON message that contains the address and the public key of a
// new participant.
type Player struct {
	Address   []byte
	PublicKey json.RawMessage
}

// ChangeSet is a JSON message of the change set of an authority.
type ChangeSet struct {
	Remove []uint32
	Add    []Player
}

// Address is a JSON message for an address.
type Address []byte

// Roster is a JSON message for a authority.
type Roster []Player

// ChangeSetFormat is the engine to encode and decode change set messages in
// JSON format.
//
// - implements serde.FormatEngine
type changeSetFormat struct{}

// Encode implements serde.FormatEngine. It returns the data serialized for the
// change set message if appropriate, otherwise an error.
func (f changeSetFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	cset, ok := msg.(authority.RosterChangeSet)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	add := make([]Player, len(cset.Add))
	for i, player := range cset.Add {
		addr, err := player.Address.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize address: %v", err)
		}

		pubkey, err := player.PublicKey.Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize public key: %v", err)
		}

		add[i] = Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}

	m := ChangeSet{
		Remove: cset.Remove,
		Add:    add,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the message with the JSON
// data if appropriate, otherwise it returns an error.
func (f changeSetFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := ChangeSet{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize change set: %v", err)
	}

	factory := ctx.GetFactory(authority.PubKeyFac{})

	pkFac, ok := factory.(crypto.PublicKeyFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid public key factory of type '%T'", factory)
	}

	factory = ctx.GetFactory(authority.AddrKeyFac{})

	addrFac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid address factory of type '%T'", factory)
	}

	if len(m.Add) == 0 {
		// Keep the addition field nil if none are present to be consistent with
		// an empty change set.
		cset := authority.RosterChangeSet{Remove: m.Remove}
		return cset, nil
	}

	add := make([]authority.Player, len(m.Add))
	for i, player := range m.Add {
		addr := addrFac.FromText(player.Address)

		pubkey, err := pkFac.PublicKeyOf(ctx, player.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize public key: %v", err)
		}

		add[i] = authority.Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}

	set := authority.RosterChangeSet{
		Remove: m.Remove,
		Add:    add,
	}

	return set, nil
}

// RosterFormat is the engine to encode and decode roster messages in JSON
// format.
//
// - implements serde.FormatEngine
type rosterFormat struct{}

// Encode implements serde.FormatEngine. It returns the data serialized for the
// roster message if appropriate, otherwise an error.
func (f rosterFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	roster, ok := msg.(authority.Roster)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	players := make([]Player, roster.Len())

	addrIter := roster.AddressIterator()
	pkIter := roster.PublicKeyIterator()
	for i := 0; addrIter.HasNext() && pkIter.HasNext(); i++ {
		addr, err := addrIter.GetNext().MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		pubkey, err := pkIter.GetNext().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize public key: %v", err)
		}

		players[i] = Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}

	m := Roster(players)

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the roster with the JSON
// data if appropriate, otherwise it returns an error.
func (f rosterFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Roster{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize roster: %v", err)
	}

	factory := ctx.GetFactory(authority.PubKeyFac{})

	pkFac, ok := factory.(crypto.PublicKeyFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid public key factory of type '%T'", factory)
	}

	factory = ctx.GetFactory(authority.AddrKeyFac{})

	addrFac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid address factory of type '%T'", factory)
	}

	addrs := make([]mino.Address, len(m))
	pubkeys := make([]crypto.PublicKey, len(m))

	for i, player := range m {
		addrs[i] = addrFac.FromText(player.Address)

		pubkey, err := pkFac.PublicKeyOf(ctx, player.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize public key: %v", err)
		}

		pubkeys[i] = pubkey
	}

	return authority.New(addrs, pubkeys), nil
}
