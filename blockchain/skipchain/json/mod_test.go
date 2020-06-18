package json

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/blockchain/skipchain/types"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func TestBlockFormat_Encode(t *testing.T) {
	block := makeBlock(t,
		types.WithIndex(5),
		types.WithGenesisID([]byte{1}),
		types.WithBackLink([]byte{2}),
		types.WithPayload(fake.Message{}))

	format := blockFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, block)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":5,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":{}}`, string(data))

	block = makeBlock(t, types.WithPayload(fakePayload{}))
	_, err = format.Encode(fake.NewBadContext(), block)
	require.EqualError(t, err, "couldn't serialize payload: fake error")
}

func TestBlockFormat_Decode(t *testing.T) {
	format := blockFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, types.PayloadKey{}, fake.MessageFactory{})

	block, err := format.Decode(ctx, []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, makeBlock(t, types.WithPayload(fake.Message{})), block)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx := serdeng.WithFactory(ctx, types.PayloadKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize payload: fake error")

	format.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err,
		"couldn't create block: couldn't write index: fake error")
}

func TestVerifiableFormat_Encode(t *testing.T) {
	vb := types.VerifiableBlock{
		SkipBlock: makeBlock(t),
		Chain:     fakeChain{},
	}

	format := newVerifiableFormat()
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, vb)
	require.NoError(t, err)
	expected := `{"Block":{"Index":0,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":null},"Chain":null}`
	require.Regexp(t, expected, string(data))

	vb.Chain = fakeChain{err: xerrors.New("oops")}
	_, err = format.Encode(ctx, vb)
	require.EqualError(t, err, "couldn't serialize chain: oops")

	format.blockFormat = badFormat{}
	_, err = format.Encode(ctx, vb)
	require.EqualError(t, err, "couldn't serialize block: oops")
}

func TestVerifiableFormat_Decode(t *testing.T) {
	format := newVerifiableFormat()
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, types.ChainKey{}, fakeChainFactory{})
	ctx = serdeng.WithFactory(ctx, types.PayloadKey{}, fake.MessageFactory{})

	block, err := format.Decode(ctx, []byte(`{"Block":{}}`))
	require.NoError(t, err)
	expected := types.VerifiableBlock{
		SkipBlock: makeBlock(t, types.WithPayload(fake.Message{})),
		Chain:     fakeChain{},
	}
	require.Equal(t, expected, block)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{"Block":{}}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx := serdeng.WithFactory(ctx, types.ChainKey{}, fakeChainFactory{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "couldn't deserialize chain: oops")

	badCtx = serdeng.WithFactory(ctx, types.PayloadKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err,
		"couldn't deserialize block: couldn't deserialize payload: fake error")
}

func TestBlueprintFormat_Encode(t *testing.T) {
	blueprint := types.NewBlueprint(5, []byte{1, 2, 3}, fake.Message{})

	format := blueprintFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, blueprint)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":5,"Previous":"[^"]+","Payload":"[^"]+"}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), blueprint)
	require.EqualError(t, err, "couldn't serialize payload: fake error")
}

func TestBlueprintFormat_Decode(t *testing.T) {
	format := blueprintFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, types.DataKey{}, fake.MessageFactory{})

	expected := types.NewBlueprint(1, []byte{2}, fake.Message{})
	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	blueprint, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.Equal(t, expected, blueprint)

	_, err = format.Decode(fake.NewBadContext(), data)
	require.EqualError(t, err, "couldn't deserialize blueprint: fake error")

	badCtx := serdeng.WithFactory(ctx, types.DataKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, data)
	require.EqualError(t, err, "couldn't deserialize payload: fake error")
}

func TestRequestFormat_Propagate_Encode(t *testing.T) {
	p := types.NewPropagateGenesis(makeBlock(t))

	format := newRequestFormat()
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, p)
	require.NoError(t, err)
	expected := `{"Genesis":{"Index":0,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":null}}`
	require.Regexp(t, expected, string(data))

	format.blockFormat = badFormat{}
	_, err = format.Encode(ctx, p)
	require.EqualError(t, err, "couldn't serialize genesis: oops")
}

func TestRequestFormat_BlockRequest_Encode(t *testing.T) {
	req := types.NewBlockRequest(1, 5)

	format := newRequestFormat()
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, req)
	require.NoError(t, err)
	require.Equal(t, `{"Request":{"From":1,"To":5}}`, string(data))
}

func TestRequestFormat_BlockResponse_Encode(t *testing.T) {
	resp := types.NewBlockResponse(makeBlock(t))

	format := newRequestFormat()
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, resp)
	require.NoError(t, err)
	expected := `{"Block":{"Index":0,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":null}}`
	require.Regexp(t, expected, string(data))

	format.blockFormat = badFormat{}
	_, err = format.Encode(ctx, resp)
	require.EqualError(t, err, "couldn't serialize block: oops")
}

func TestRequestFormat_Decode(t *testing.T) {
	format := newRequestFormat()
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, types.PayloadKey{}, fake.MessageFactory{})

	msg, err := format.Decode(ctx, []byte(`{"Propagate":{"Genesis":{}}}`))
	require.NoError(t, err)
	expected := types.NewPropagateGenesis(makeBlock(t, types.WithPayload(fake.Message{})))
	require.Equal(t, expected, msg)

	badCtx := serdeng.WithFactory(ctx, types.PayloadKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Propagate":{"Genesis":{}}}`))
	require.EqualError(t, err,
		"couldn't deserialize genesis: couldn't deserialize payload: fake error")

	req, err := format.Decode(ctx, []byte(`{"Request":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewBlockRequest(0, 0), req)

	resp, err := format.Decode(ctx, []byte(`{"Response":{"Block":{}}}`))
	require.NoError(t, err)
	expected2 := types.NewBlockResponse(makeBlock(t, types.WithPayload(fake.Message{})))
	require.Equal(t, expected2, resp)

	_, err = format.Decode(badCtx, []byte(`{"Response":{"Block":{}}}`))
	require.EqualError(t, err,
		"couldn't deserialize block: couldn't deserialize payload: fake error")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeBlock(t *testing.T, opts ...types.SkipBlockOption) types.SkipBlock {
	block, err := types.NewSkipBlock(opts...)
	require.NoError(t, err)
	return block
}

type fakePayload struct {
	blockchain.Payload
	err error
}

func (p fakePayload) Serialize(ctx serdeng.Context) ([]byte, error) {
	return ctx.Marshal(struct{}{})
}

func (p fakePayload) Fingerprint(io.Writer) error {
	return p.err
}

type fakeChain struct {
	consensus.Chain

	err error
}

func (c fakeChain) Serialize(serdeng.Context) ([]byte, error) {
	return nil, c.err
}

type fakeChainFactory struct {
	consensus.ChainFactory

	err error
}

func (f fakeChainFactory) ChainOf(serdeng.Context, []byte) (consensus.Chain, error) {
	return fakeChain{}, f.err
}

type badFormat struct{}

func (f badFormat) Encode(serdeng.Context, serdeng.Message) ([]byte, error) {
	return nil, xerrors.New("oops")
}

func (f badFormat) Decode(serdeng.Context, []byte) (serdeng.Message, error) {
	return nil, xerrors.New("oops")
}
