package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterGenesisFormat(serde.FormatJSON, genesisFormat{})
	types.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
	types.RegisterBlockFormat(serde.FormatJSON, blockFormat{})
	types.RegisterLinkFormat(serde.FormatJSON, linkFormat{})
	types.RegisterChainFormat(serde.FormatJSON, chainFormat{})
}

// GenesisJSON is the JSON message for a genesis block.
type GenesisJSON struct {
	Roster   json.RawMessage
	TreeRoot []byte
}

// BlockJSON is the JSON message for a block.
type BlockJSON struct {
	Index    uint64
	TreeRoot []byte
	Data     json.RawMessage
}

// LinkJSON is the JSON message for a link.
type LinkJSON struct {
	From             []byte
	To               []byte `json:",omitempty"`
	PrepareSignature json.RawMessage
	CommitSignature  json.RawMessage
	ChangeSet        json.RawMessage
	Block            json.RawMessage `json:",omitempty"`
}

// ChainJSON is the JSON message for a chain.
type ChainJSON struct {
	Links []json.RawMessage
}

// GenesisMessageJSON is the JSON message to send a genesis block.
type GenesisMessageJSON struct {
	Genesis json.RawMessage
}

// BlockMessageJSON is the JSON message to send a block.
type BlockMessageJSON struct {
	Block json.RawMessage
	Views map[string]ViewMessageJSON
}

// CommitMessageJSON is the JSON message to send a commit request.
type CommitMessageJSON struct {
	ID        []byte
	Signature json.RawMessage
}

// DoneMessageJSON is the JSON message to send a block confirmation.
type DoneMessageJSON struct {
	ID        []byte
	Signature json.RawMessage
}

// ViewMessageJSON is the JSON message to send a view change request.
type ViewMessageJSON struct {
	Leader    uint16
	ID        []byte
	Signature json.RawMessage
}

// MessageJSON is the JSON message that wraps the different kinds of messages.
type MessageJSON struct {
	Genesis *GenesisMessageJSON `json:",omitempty"`
	Block   *BlockMessageJSON   `json:",omitempty"`
	Commit  *CommitMessageJSON  `json:",omitempty"`
	Done    *DoneMessageJSON    `json:",omitempty"`
	View    *ViewMessageJSON    `json:",omitempty"`
}

// GenesisFormat is a format engine to serialize and deserialize the genesis
// blocks.
//
// - implements serde.FormatEngine
type genesisFormat struct {
	hashFac crypto.HashFactory
}

// Encode implements serde.FormatEngine. It returns the serialized data of the
// genesis if appropriate, otherwise it returns an error.
func (f genesisFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	genesis, ok := msg.(types.Genesis)
	if !ok {
		return nil, xerrors.Errorf("invalid genesis '%T'", msg)
	}

	roster, err := genesis.GetRoster().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize roster: %v", err)
	}

	m := GenesisJSON{
		Roster:   roster,
		TreeRoot: genesis.GetRoot().Bytes(),
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the genesis block if
// appropriate, otherwise it returns an error.
func (f genesisFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := GenesisJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	factory := ctx.GetFactory(types.RosterKey{})

	fac, ok := factory.(authority.Factory)
	if !ok {
		return nil, xerrors.Errorf("invalid roster factory '%T'", factory)
	}

	roster, err := fac.AuthorityOf(ctx, m.Roster)
	if err != nil {
		return nil, xerrors.Errorf("authority factory failed: %v", err)
	}

	root := types.Digest{}
	copy(root[:], m.TreeRoot)

	opts := []types.GenesisOption{types.WithGenesisRoot(root)}

	if f.hashFac != nil {
		opts = append(opts, types.WithGenesisHashFactory(f.hashFac))
	}

	genesis, err := types.NewGenesis(roster, opts...)
	if err != nil {
		return nil, xerrors.Errorf("creating genesis: %v", err)
	}

	return genesis, nil
}

// BlockFormat is the format engine to serialize and deserialize the blocks.
//
// - implements serde.FormatEngine
type blockFormat struct {
	hashFac crypto.HashFactory
}

// Encode implements serde.FormatEngine. It returns the serialized data of the
// block if appropritate, otherwise it returns an error.
func (f blockFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	block, ok := msg.(types.Block)
	if !ok {
		return nil, xerrors.Errorf("invalid block '%T'", msg)
	}

	blockdata, err := block.GetData().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize data: %v", err)
	}

	m := BlockJSON{
		Index:    block.GetIndex(),
		TreeRoot: block.GetTreeRoot().Bytes(),
		Data:     blockdata,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the block if appropriate,
// otherwise it returns an error.
func (f blockFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := BlockJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	factory := ctx.GetFactory(types.DataKey{})

	fac, ok := factory.(validation.ResultFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid data factory '%T'", factory)
	}

	blockdata, err := fac.ResultOf(ctx, m.Data)
	if err != nil {
		return nil, xerrors.Errorf("data factory failed: %v", err)
	}

	root := types.Digest{}
	copy(root[:], m.TreeRoot)

	opts := []types.BlockOption{
		types.WithTreeRoot(root),
		types.WithIndex(m.Index),
	}

	if f.hashFac != nil {
		opts = append(opts, types.WithHashFactory(f.hashFac))
	}

	block, err := types.NewBlock(blockdata, opts...)
	if err != nil {
		return nil, xerrors.Errorf("creating block: %v", err)
	}

	return block, nil
}

// MsgFormat is the format engine to serialize and deserialize the messages.
//
// - implements serde.FormatEngine
type msgFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data of the
// message if appropriate, otherwise it returns an error.
func (f msgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m MessageJSON

	switch in := msg.(type) {
	case types.GenesisMessage:
		genesis, err := in.GetGenesis().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed to serialize genesis: %v", err)
		}

		gm := GenesisMessageJSON{
			Genesis: genesis,
		}

		m = MessageJSON{Genesis: &gm}
	case types.BlockMessage:
		block, err := in.GetBlock().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("block: %v", err)
		}

		views := make(map[string]ViewMessageJSON)
		for addr, view := range in.GetViews() {
			key, err := addr.MarshalText()
			if err != nil {
				return nil, xerrors.Errorf("failed to serialize address: %v", err)
			}

			rawView, err := encodeView(view, ctx)
			if err != nil {
				return nil, xerrors.Errorf("view: %v", err)
			}

			views[string(key)] = *rawView
		}

		bm := BlockMessageJSON{
			Block: block,
			Views: views,
		}

		m = MessageJSON{Block: &bm}
	case types.CommitMessage:
		sig, err := in.GetSignature().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed to serialize signature: %v", err)
		}

		cm := CommitMessageJSON{
			ID:        in.GetID().Bytes(),
			Signature: sig,
		}

		m = MessageJSON{Commit: &cm}
	case types.DoneMessage:
		sig, err := in.GetSignature().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed to serialize signature: %v", err)
		}

		dm := DoneMessageJSON{
			ID:        in.GetID().Bytes(),
			Signature: sig,
		}

		m = MessageJSON{Done: &dm}
	case types.ViewMessage:
		vm, err := encodeView(in, ctx)
		if err != nil {
			return nil, xerrors.Errorf("view: %v", err)
		}

		m = MessageJSON{View: vm}
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

func encodeView(in types.ViewMessage, ctx serde.Context) (*ViewMessageJSON, error) {
	sig, err := in.GetSignature().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize signature: %v", err)
	}

	vm := &ViewMessageJSON{
		ID:        in.GetID().Bytes(),
		Leader:    in.GetLeader(),
		Signature: sig,
	}

	return vm, nil
}

// Decode implements serde.FormatEngine. It populates the message if
// appropriate, otherwise it returns an error.
func (f msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := MessageJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	if m.Genesis != nil {
		factory := ctx.GetFactory(types.GenesisKey{})
		if factory == nil {
			return nil, xerrors.New("missing genesis factory")
		}

		msg, err := factory.Deserialize(ctx, m.Genesis.Genesis)
		if err != nil {
			return nil, xerrors.Errorf("failed to deserialize genesis: %v", err)
		}

		genesis, ok := msg.(types.Genesis)
		if !ok {
			return nil, xerrors.Errorf("invalid genesis '%T'", msg)
		}

		return types.NewGenesisMessage(genesis), nil
	}

	if m.Block != nil {
		// 1. Decode the block.
		factory := ctx.GetFactory(types.BlockKey{})
		if factory == nil {
			return nil, xerrors.New("missing block factory")
		}

		msg, err := factory.Deserialize(ctx, m.Block.Block)
		if err != nil {
			return nil, xerrors.Errorf("failed to deserialize block: %v", err)
		}

		block, ok := msg.(types.Block)
		if !ok {
			return nil, xerrors.Errorf("invalid block '%T'", msg)
		}

		// 2. Decode the view messages if any.
		factory = ctx.GetFactory(types.AddressKey{})

		fac, ok := factory.(mino.AddressFactory)
		if !ok {
			return nil, xerrors.Errorf("invalid address factory '%T'", factory)
		}

		views := make(map[mino.Address]types.ViewMessage)
		for key, rawView := range m.Block.Views {
			addr := fac.FromText([]byte(key))

			view, err := decodeView(ctx, &rawView)
			if err != nil {
				return nil, xerrors.Errorf("view: %v", err)
			}

			views[addr] = view
		}

		return types.NewBlockMessage(block, views), nil
	}

	if m.Commit != nil {
		sig, err := decodeSignature(ctx, m.Commit.Signature, types.AggregateKey{})
		if err != nil {
			return nil, xerrors.Errorf("commit failed: %v", err)
		}

		id := types.Digest{}
		copy(id[:], m.Commit.ID)

		return types.NewCommit(id, sig), nil
	}

	if m.Done != nil {
		sig, err := decodeSignature(ctx, m.Done.Signature, types.AggregateKey{})
		if err != nil {
			return nil, xerrors.Errorf("done failed: %v", err)
		}

		id := types.Digest{}
		copy(id[:], m.Done.ID)

		return types.NewDone(id, sig), nil
	}

	if m.View != nil {
		return decodeView(ctx, m.View)
	}

	return nil, xerrors.New("message is empty")
}

func decodeView(ctx serde.Context, view *ViewMessageJSON) (types.ViewMessage, error) {
	sig, err := decodeSignature(ctx, view.Signature, types.SignatureKey{})
	if err != nil {
		return types.ViewMessage{}, xerrors.Errorf("signature: %v", err)
	}

	id := types.Digest{}
	copy(id[:], view.ID)

	return types.NewViewMessage(id, view.Leader, sig), nil
}

func decodeSignature(ctx serde.Context, data []byte, key interface{}) (crypto.Signature, error) {
	factory := ctx.GetFactory(key)

	fac, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid signature factory '%T'", factory)
	}

	sig, err := fac.SignatureOf(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("factory failed: %v", err)
	}

	return sig, nil
}
