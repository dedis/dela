package json

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func init() {
	types.RegisterGenesisFormat(fake.GoodFormat, fakeGenesisFormat{})
	types.RegisterGenesisFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestGenesisFormat_Encode(t *testing.T) {
	format := genesisFormat{}

	ctx := fake.NewContext()

	genesis, err := types.NewGenesis(fakeRoster{}, types.WithGenesisRoot(types.Digest{1}))
	require.NoError(t, err)

	data, err := format.Encode(ctx, genesis)
	require.NoError(t, err)
	require.Regexp(t, `{"Roster":{},"TreeRoot":"[^"]+"}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "invalid genesis 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), genesis)
	require.EqualError(t, err, fake.Err("failed to marshal"))

	genesis, err = types.NewGenesis(fakeRoster{err: fake.GetError()})
	require.NoError(t, err)

	_, err = format.Encode(ctx, genesis)
	require.EqualError(t, err, fake.Err("failed to serialize roster"))
}

func TestGenesisFormat_Decode(t *testing.T) {
	format := genesisFormat{}

	genesis, err := types.NewGenesis(fakeRoster{})
	require.NoError(t, err)

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.RosterKey{}, fakeRosterFac{})

	msg, err := format.Decode(ctx, []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, msg, genesis)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	badCtx := serde.WithFactory(ctx, types.RosterKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid roster factory '<nil>'")

	badCtx = serde.WithFactory(ctx, types.RosterKey{}, fakeRosterFac{err: fake.GetError()})
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("authority factory failed"))

	format.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating genesis: fingerprint failed: ")
}

func TestBlockFormat_Encode(t *testing.T) {
	format := blockFormat{}

	ctx := fake.NewContext()

	block, err := types.NewBlock(fakeResult{})
	require.NoError(t, err)

	data, err := format.Encode(ctx, block)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":0,"TreeRoot":"[^"]+","Data":{}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "invalid block 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), block)
	require.EqualError(t, err, fake.Err("failed to marshal"))

	block, err = types.NewBlock(fakeResult{err: fake.GetError()})
	require.NoError(t, err)

	_, err = format.Encode(ctx, block)
	require.EqualError(t, err, fake.Err("failed to serialize data"))
}

func TestBlockFormat_Decode(t *testing.T) {
	format := blockFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.DataKey{}, fakeResultFac{})

	block, err := types.NewBlock(fakeResult{})
	require.NoError(t, err)

	msg, err := format.Decode(ctx, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, block, msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	badCtx := serde.WithFactory(ctx, types.DataKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid data factory '<nil>'")

	badCtx = serde.WithFactory(ctx, types.DataKey{}, fakeResultFac{err: fake.GetError()})
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("data factory failed"))

	format.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating block: fingerprint failed: ")
}

func TestMsgFormat_Encode(t *testing.T) {
	format := msgFormat{}

	genesis, err := types.NewGenesis(fakeRoster{})
	require.NoError(t, err)

	block, err := types.NewBlock(fakeResult{})
	require.NoError(t, err)

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, types.NewGenesisMessage(genesis))
	require.NoError(t, err)
	require.Equal(t, `{"Genesis":{"Genesis":{}}}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), types.NewGenesisMessage(genesis))
	require.EqualError(t, err, fake.Err("failed to serialize genesis: encoding failed"))

	views := map[mino.Address]types.ViewMessage{
		fake.NewAddress(0): types.NewViewMessage(types.Digest{1}, 5, fake.Signature{}),
	}
	data, err = format.Encode(ctx, types.NewBlockMessage(block, views))
	require.NoError(t, err)
	require.Regexp(t,
		`{"Block":{"Block":{},"Views":{"[^"]+":{"Leader":5,"ID":"[^"]+","Signature":{}}}}}`, string(data))

	views[fake.NewAddress(0)] = types.NewViewMessage(types.Digest{}, 0, fake.NewBadSignature())
	_, err = format.Encode(ctx, types.NewBlockMessage(block, views))
	require.EqualError(t, err, fake.Err("view: failed to serialize signature"))

	delete(views, fake.NewAddress(0))
	views[fake.NewBadAddress()] = types.NewViewMessage(types.Digest{}, 0, fake.Signature{})
	_, err = format.Encode(ctx, types.NewBlockMessage(block, views))
	require.EqualError(t, err, fake.Err("failed to serialize address"))

	_, err = format.Encode(fake.NewBadContext(), types.NewBlockMessage(block, nil))
	require.EqualError(t, err, fake.Err("block: encoding failed"))

	data, err = format.Encode(ctx, types.NewCommit(types.Digest{}, fake.Signature{}))
	require.NoError(t, err)
	require.Regexp(t, `{"Commit":{"ID":"[^"]+","Signature":{}}}`, string(data))

	_, err = format.Encode(ctx, types.NewCommit(types.Digest{}, fake.NewBadSignature()))
	require.EqualError(t, err, fake.Err("failed to serialize signature"))

	data, err = format.Encode(ctx, types.NewDone(types.Digest{}, fake.Signature{}))
	require.NoError(t, err)
	require.Regexp(t, `{"Done":{"ID":"[^"]+","Signature":{}}}`, string(data))

	_, err = format.Encode(ctx, types.NewDone(types.Digest{}, fake.NewBadSignature()))
	require.EqualError(t, err, fake.Err("failed to serialize signature"))

	data, err = format.Encode(ctx, types.NewViewMessage(types.Digest{}, 5, fake.Signature{}))
	require.NoError(t, err)
	require.Regexp(t, `{"View":{"Leader":5,"ID":"[^"]+","Signature":{}}}`, string(data))

	_, err = format.Encode(ctx, types.NewViewMessage(types.Digest{}, 0, fake.NewBadSignature()))
	require.EqualError(t, err, fake.Err("view: failed to serialize signature"))

	_, err = format.Encode(fake.NewBadContext(), types.NewViewMessage(types.Digest{}, 0, fake.Signature{}))
	require.EqualError(t, err, fake.Err("failed to marshal"))
}

func TestMsgFormat_Decode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.GenesisKey{}, types.GenesisFactory{})
	ctx = serde.WithFactory(ctx, types.BlockKey{}, types.BlockFactory{})
	ctx = serde.WithFactory(ctx, types.AggregateKey{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.SignatureKey{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.AddressKey{}, fake.AddressFactory{})

	msg, err := format.Decode(ctx, []byte(`{"Genesis":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.GenesisMessage{}, msg)

	badCtx := serde.WithFactory(ctx, types.GenesisKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Genesis":{}}`))
	require.EqualError(t, err, "missing genesis factory")

	badCtx = serde.WithFactory(ctx, types.GenesisKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Genesis":{}}`))
	require.EqualError(t, err, fake.Err("failed to deserialize genesis"))

	badCtx = serde.WithFactory(ctx, types.GenesisKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Genesis":{}}`))
	require.EqualError(t, err, "invalid genesis 'fake.Message'")

	msg, err = format.Decode(ctx, []byte(`{"Block":{"Views":{"":{}}}}`))
	require.NoError(t, err)
	require.IsType(t, types.BlockMessage{}, msg)
	require.Len(t, msg.(types.BlockMessage).GetViews(), 1)

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "missing block factory")

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, fake.Err("failed to deserialize block"))

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "invalid block 'fake.Message'")

	badCtx = serde.WithFactory(ctx, types.AddressKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Block":{"Views":{"":{}}}}`))
	require.EqualError(t, err, "invalid address factory '<nil>'")

	badCtx = serde.WithFactory(ctx, types.SignatureKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Block":{"Views":{"":{}}}}`))
	require.EqualError(t, err, "view: signature: invalid signature factory '<nil>'")

	msg, err = format.Decode(ctx, []byte(`{"Commit":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.CommitMessage{}, msg)

	badCtx = serde.WithFactory(ctx, types.AggregateKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Commit":{}}`))
	require.EqualError(t, err, "commit failed: invalid signature factory '<nil>'")

	msg, err = format.Decode(ctx, []byte(`{"Done":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.DoneMessage{}, msg)

	_, err = format.Decode(badCtx, []byte(`{"Done":{}}`))
	require.EqualError(t, err, "done failed: invalid signature factory '<nil>'")

	msg, err = format.Decode(ctx, []byte(`{"View":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.ViewMessage{}, msg)

	badCtx = serde.WithFactory(ctx, types.SignatureKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"View":{}}`))
	require.EqualError(t, err, "signature: invalid signature factory '<nil>'")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeRoster struct {
	authority.Authority

	err error
}

func (ro fakeRoster) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), ro.err
}

func (fakeRoster) Fingerprint(io.Writer) error {
	return nil
}

type fakeRosterFac struct {
	authority.Factory

	err error
}

func (fac fakeRosterFac) AuthorityOf(serde.Context, []byte) (authority.Authority, error) {
	return fakeRoster{}, fac.err
}

type fakeGenesisFormat struct {
	serde.FormatEngine
}

func (fakeGenesisFormat) Encode(serde.Context, serde.Message) ([]byte, error) {
	return []byte(`{}`), nil
}

func (fakeGenesisFormat) Decode(serde.Context, []byte) (serde.Message, error) {
	genesis, err := types.NewGenesis(authority.New(nil, nil))
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
