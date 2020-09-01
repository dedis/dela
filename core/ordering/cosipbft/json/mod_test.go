package json

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
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
	require.EqualError(t, err, "failed to marshal: fake error")

	genesis, err = types.NewGenesis(fakeRoster{err: xerrors.New("oops")})
	require.NoError(t, err)

	_, err = format.Encode(ctx, genesis)
	require.EqualError(t, err, "failed to serialize roster: oops")
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
	require.EqualError(t, err, "failed to unmarshal: fake error")

	badCtx := serde.WithFactory(ctx, types.RosterKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid roster factory '<nil>'")

	badCtx = serde.WithFactory(ctx, types.RosterKey{}, fakeRosterFac{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "authority factory failed: oops")

	format.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating genesis: fingerprint failed: ")
}

func TestBlockFormat_Encode(t *testing.T) {
	format := blockFormat{}

	ctx := fake.NewContext()

	block, err := types.NewBlock(fakeData{})
	require.NoError(t, err)

	data, err := format.Encode(ctx, block)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":0,"TreeRoot":"[^"]+","Data":{}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "invalid block 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), block)
	require.EqualError(t, err, "failed to marshal: fake error")

	block, err = types.NewBlock(fakeData{err: xerrors.New("oops")})
	require.NoError(t, err)

	_, err = format.Encode(ctx, block)
	require.EqualError(t, err, "failed to serialize data: oops")
}

func TestBlockFormat_Decode(t *testing.T) {
	format := blockFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.DataKey{}, fakeDataFac{})

	block, err := types.NewBlock(fakeData{})
	require.NoError(t, err)

	msg, err := format.Decode(ctx, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, block, msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "failed to unmarshal: fake error")

	badCtx := serde.WithFactory(ctx, types.DataKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid data factory '<nil>'")

	badCtx = serde.WithFactory(ctx, types.DataKey{}, fakeDataFac{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "data factory failed: oops")

	format.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating block: fingerprint failed: ")
}

func TestMsgFormat_Encode(t *testing.T) {
	format := msgFormat{}

	genesis, err := types.NewGenesis(fakeRoster{})
	require.NoError(t, err)

	block, err := types.NewBlock(fakeData{})
	require.NoError(t, err)

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, types.NewGenesisMessage(genesis))
	require.NoError(t, err)
	require.Equal(t, `{"Genesis":{"Genesis":{}}}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), types.NewGenesisMessage(genesis))
	require.EqualError(t, err, "failed to serialize genesis: encoding failed: fake error")

	data, err = format.Encode(ctx, types.NewBlockMessage(block))
	require.NoError(t, err)
	require.Equal(t, `{"Block":{"Block":{}}}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), types.NewBlockMessage(block))
	require.EqualError(t, err, "failed to serialize block: encoding failed: fake error")

	data, err = format.Encode(ctx, types.NewCommit(types.Digest{}, fake.Signature{}))
	require.NoError(t, err)
	require.Regexp(t, `{"Commit":{"ID":"[^"]+","Signature":{}}}`, string(data))

	_, err = format.Encode(ctx, types.NewCommit(types.Digest{}, fake.NewBadSignature()))
	require.EqualError(t, err, "failed to serialize signature: fake error")

	data, err = format.Encode(ctx, types.NewDone(types.Digest{}, fake.Signature{}))
	require.NoError(t, err)
	require.Regexp(t, `{"Done":{"ID":"[^"]+","Signature":{}}}`, string(data))

	_, err = format.Encode(ctx, types.NewDone(types.Digest{}, fake.NewBadSignature()))
	require.EqualError(t, err, "failed to serialize signature: fake error")

	data, err = format.Encode(ctx, types.NewViewMessage(types.Digest{}, 5))
	require.NoError(t, err)
	require.Regexp(t, `{"View":{"Leader":5,"ID":"[^"]+"}}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), types.NewViewMessage(types.Digest{}, 0))
	require.EqualError(t, err, "failed to marshal: fake error")
}

func TestMsgFormat_Decode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.GenesisKey{}, types.GenesisFactory{})
	ctx = serde.WithFactory(ctx, types.BlockKey{}, types.BlockFactory{})
	ctx = serde.WithFactory(ctx, types.SignatureKey{}, fake.SignatureFactory{})

	msg, err := format.Decode(ctx, []byte(`{"Genesis":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.GenesisMessage{}, msg)

	badCtx := serde.WithFactory(ctx, types.GenesisKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Genesis":{}}`))
	require.EqualError(t, err, "missing genesis factory")

	badCtx = serde.WithFactory(ctx, types.GenesisKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Genesis":{}}`))
	require.EqualError(t, err, "failed to deserialize genesis: fake error")

	badCtx = serde.WithFactory(ctx, types.GenesisKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Genesis":{}}`))
	require.EqualError(t, err, "invalid genesis 'fake.Message'")

	msg, err = format.Decode(ctx, []byte(`{"Block":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.BlockMessage{}, msg)

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "missing block factory")

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "failed to deserialize block: fake error")

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "invalid block 'fake.Message'")

	msg, err = format.Decode(ctx, []byte(`{"Commit":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.CommitMessage{}, msg)

	badCtx = serde.WithFactory(ctx, types.SignatureKey{}, nil)
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

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "failed to unmarshal: fake error")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}

// Utility functions
// -----------------------------------------------------------------------------

type fakeRoster struct {
	viewchange.Authority

	err error
}

func (ro fakeRoster) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), ro.err
}

func (fakeRoster) Fingerprint(io.Writer) error {
	return nil
}

type fakeRosterFac struct {
	viewchange.AuthorityFactory

	err error
}

func (fac fakeRosterFac) AuthorityOf(serde.Context, []byte) (viewchange.Authority, error) {
	return fakeRoster{}, fac.err
}

type fakeGenesisFormat struct {
	serde.FormatEngine
}

func (fakeGenesisFormat) Encode(serde.Context, serde.Message) ([]byte, error) {
	return []byte(`{}`), nil
}

func (fakeGenesisFormat) Decode(serde.Context, []byte) (serde.Message, error) {
	genesis, err := types.NewGenesis(roster.New(nil, nil))
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
