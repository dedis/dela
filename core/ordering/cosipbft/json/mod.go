package json

import (
	"encoding/json"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterGenesisFormat(serde.FormatJSON, genesisFormat{})
	types.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
	types.RegisterBlockFormat(serde.FormatJSON, blockFormat{})
	types.RegisterLinkFormat(serde.FormatJSON, linkFormat{})
}

type GenesisJSON struct {
	Roster json.RawMessage
}

type BlockJSON struct {
	Index    uint64
	TreeRoot types.Digest
	Data     json.RawMessage
}

type BlockLinkJSON struct {
	From             types.Digest
	Block            json.RawMessage
	PrepareSignature json.RawMessage
	CommitSignature  json.RawMessage
}

type GenesisMessageJSON struct {
	Genesis json.RawMessage
}

type BlockMessageJSON struct {
	Block json.RawMessage
}

type CommitMessageJSON struct {
	ID        types.Digest
	Signature json.RawMessage
}

type DoneMessageJSON struct {
	ID        types.Digest
	Signature json.RawMessage
}

type ViewMessageJSON struct {
	Leader int
	ID     types.Digest
}

type MessageJSON struct {
	Genesis *GenesisMessageJSON
	Block   *BlockMessageJSON
	Commit  *CommitMessageJSON
	Done    *DoneMessageJSON
	View    *ViewMessageJSON
}

type genesisFormat struct{}

func (f genesisFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	genesis, ok := msg.(types.Genesis)
	if !ok {
		return nil, xerrors.Errorf("invalid genesis '%T'", msg)
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

	genesis, err := types.NewGenesis(roster)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}

type blockFormat struct{}

func (f blockFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	block, ok := msg.(types.Block)
	if !ok {
		return nil, xerrors.New("invalid block type")
	}

	blockdata, err := block.GetData().Serialize(ctx)
	if err != nil {
		return nil, err
	}

	m := BlockJSON{
		Index:    block.GetIndex(),
		TreeRoot: block.GetTreeRoot(),
		Data:     blockdata,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f blockFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := BlockJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	factory := ctx.GetFactory(types.DataKey{})

	fac, ok := factory.(validation.DataFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid data factory")
	}

	blockdata, err := fac.DataOf(ctx, m.Data)
	if err != nil {
		return nil, err
	}

	block, err := types.NewBlock(
		blockdata,
		types.WithTreeRoot(m.TreeRoot),
		types.WithIndex(m.Index),
	)

	if err != nil {
		return nil, err
	}

	return block, nil
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
		block, err := in.GetBlock().Serialize(ctx)
		if err != nil {
			return nil, err
		}

		bm := BlockMessageJSON{
			Block: block,
		}

		m = MessageJSON{Block: &bm}
	case types.CommitMessage:
		sig, err := in.GetSignature().Serialize(ctx)
		if err != nil {
			return nil, err
		}

		cm := CommitMessageJSON{
			ID:        in.GetID(),
			Signature: sig,
		}

		m = MessageJSON{Commit: &cm}
	case types.DoneMessage:
		sig, err := in.GetSignature().Serialize(ctx)
		if err != nil {
			return nil, err
		}

		dm := DoneMessageJSON{
			ID:        in.GetID(),
			Signature: sig,
		}

		m = MessageJSON{Done: &dm}
	case types.ViewMessage:
		vm := ViewMessageJSON{
			ID:     in.GetID(),
			Leader: in.GetLeader(),
		}

		m = MessageJSON{View: &vm}
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
		factory := ctx.GetFactory(types.BlockKey{})

		msg, err := factory.Deserialize(ctx, m.Block.Block)
		if err != nil {
			return nil, err
		}

		block, ok := msg.(types.Block)
		if !ok {
			return nil, xerrors.New("invalid block")
		}

		return types.NewBlockMessage(block), nil
	}

	if m.Commit != nil {
		sig, err := decodeSignature(ctx, m.Commit.Signature)
		if err != nil {
			return nil, xerrors.Errorf("commit failed: %v", err)
		}

		return types.NewCommit(m.Commit.ID, sig), nil
	}

	if m.Done != nil {
		sig, err := decodeSignature(ctx, m.Done.Signature)
		if err != nil {
			return nil, xerrors.Errorf("done failed: %v", err)
		}

		return types.NewDone(m.Done.ID, sig), nil
	}

	if m.View != nil {
		return types.NewViewMessage(m.View.ID, m.View.Leader), nil
	}

	return nil, xerrors.New("message is empty")
}

func decodeSignature(ctx serde.Context, data []byte) (crypto.Signature, error) {
	factory := ctx.GetFactory(types.SignatureKey{})

	fac, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid signature")
	}

	sig, err := fac.SignatureOf(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("factory failed: %v", err)
	}

	return sig, nil
}
