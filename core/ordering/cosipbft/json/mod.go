package json

import (
	"encoding/json"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterGenesisFormat(serde.FormatJSON, genesisFormat{})
	types.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
}

type GenesisJSON struct {
	Roster json.RawMessage
}

type GenesisMessageJSON struct {
	Genesis json.RawMessage
}

type BlockMessageJSON struct{}

type CommitMessageJSON struct{}

type DoneMessageJSON struct{}

type MessageJSON struct {
	Genesis *GenesisMessageJSON
	Block   *BlockMessageJSON
	Commit  *CommitMessageJSON
	Done    *DoneMessageJSON
}

type genesisFormat struct{}

func (f genesisFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	genesis, ok := msg.(types.Genesis)
	if !ok {
		return nil, xerrors.New("invalid genesis")
	}

	roster, err := genesis.GetRoster().Serialize(ctx)
	if err != nil {
		return nil, err
	}

	m := GenesisJSON{
		Roster: roster,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f genesisFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := GenesisJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	factory := ctx.GetFactory(types.RosterKey{})

	fac, ok := factory.(viewchange.AuthorityFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid roster factory '%T'", factory)
	}

	roster, err := fac.AuthorityOf(ctx, m.Roster)
	if err != nil {
		return nil, err
	}

	return types.NewGenesis(roster), nil
}

type msgFormat struct{}

func (f msgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m MessageJSON

	switch in := msg.(type) {
	case types.GenesisMessage:
		genesis, err := in.GetGenesis().Serialize(ctx)
		if err != nil {
			return nil, err
		}

		gm := GenesisMessageJSON{
			Genesis: genesis,
		}

		m = MessageJSON{Genesis: &gm}
	case types.BlockMessage:
		bm := BlockMessageJSON{}

		m = MessageJSON{Block: &bm}
	case types.CommitMessage:
		cm := CommitMessageJSON{}

		m = MessageJSON{Commit: &cm}
	case types.DoneMessage:
		dm := DoneMessageJSON{}

		m = MessageJSON{Done: &dm}
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := MessageJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	if m.Genesis != nil {
		factory := ctx.GetFactory(types.GenesisKey{})

		msg, err := factory.Deserialize(ctx, m.Genesis.Genesis)
		if err != nil {
			return nil, err
		}

		genesis, ok := msg.(types.Genesis)
		if !ok {
			return nil, xerrors.New("invalid genesis type")
		}

		return types.NewGenesisMessage(genesis), nil
	}

	if m.Block != nil {
		return types.BlockMessage{}, nil
	}

	if m.Commit != nil {
		return types.CommitMessage{}, nil
	}

	if m.Done != nil {
		return types.DoneMessage{}, nil
	}

	return nil, xerrors.New("message is empty")
}
