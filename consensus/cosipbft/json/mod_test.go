package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/cosipbft/types"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestLinkFormat_Encode(t *testing.T) {
	fl := makeFL(t)

	format := linkFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, fl)
	require.NoError(t, err)
	expected := `{"From":"AQ==","To":"Ag==","Prepare":{},"Commit":{},"ChangeSet":{}}`
	require.Equal(t, expected, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), fl)
	require.EqualError(t, err, "couldn't marshal: fake error")

	fl = makeFL(t, types.WithChangeSet(fakeChangeSet{err: xerrors.New("oops")}))
	_, err = format.Encode(ctx, fl)
	require.EqualError(t, err, "couldn't serialize changeset: oops")

	fl = makeFL(t, types.WithPrepare(fake.NewBadSignature()))
	_, err = format.Encode(ctx, fl)
	require.EqualError(t, err, "couldn't serialize prepare signature: fake error")

	fl = makeFL(t, types.WithCommit(fake.NewBadSignature()))
	_, err = format.Encode(ctx, fl)
	require.EqualError(t, err, "couldn't serialize commit signature: fake error")
}

func TestLinkFormat_Decode(t *testing.T) {
	format := linkFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)
	ctx = serde.WithFactory(ctx, types.CoSigKeyFac{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.ChangeSetKeyFac{}, fakeChangeSetFactory{})

	msg, err := format.Decode(ctx, []byte(`{"From":"AQ==","To":"Ag=="}`))
	require.NoError(t, err)
	require.Equal(t, makeFL(t), msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't unmarshal link: fake error")

	badCtx := serde.WithFactory(ctx, types.CoSigKeyFac{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize prepare: fake error")

	badCtx = serde.WithFactory(ctx, types.CoSigKeyFac{}, fake.NewBadSignatureFactoryWithDelay(1))
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize commit: fake error")

	badCtx = serde.WithFactory(ctx, types.CoSigKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid signature factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx,
		types.ChangeSetKeyFac{}, fakeChangeSetFactory{err: xerrors.New("oops")})

	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize change set: oops")

	badCtx = serde.WithFactory(ctx, types.ChangeSetKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid change set factory of type '<nil>'")
}

func TestChainFormat_Encode(t *testing.T) {
	chain := types.NewChain(makeFL(t))

	format := chainFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, chain)
	require.NoError(t, err)
	expected := `[{"From":"AQ==","To":"Ag==","Prepare":{},"Commit":{},"ChangeSet":{}}]`
	require.Equal(t, expected, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), chain)
	require.EqualError(t, err, "couldn't marshal: fake error")

	chain = types.NewChain(makeFL(t, types.WithPrepare(fake.NewBadSignature())))
	_, err = format.Encode(ctx, chain)
	require.EqualError(t, err,
		"couldn't serialize link: couldn't serialize prepare signature: fake error")
}

func TestChainFormat_Decode(t *testing.T) {
	expected := types.NewChain(makeFL(t), makeFL(t))

	format := chainFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)
	ctx = serde.WithFactory(ctx, types.CoSigKeyFac{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.ChangeSetKeyFac{}, fakeChangeSetFactory{})

	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	chain, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.Equal(t, expected, chain)

	_, err = format.Decode(fake.NewBadContext(), data)
	require.EqualError(t, err, "couldn't deserialize chain: fake error")
}

func TestMessageFormat_Prepare_Encode(t *testing.T) {
	prepare := types.NewPrepare(fake.Message{}, fake.Signature{}, types.NewChain())

	format := requestFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, prepare)
	require.NoError(t, err)
	expected := `{"Prepare":{"Message":{},"Signature":{},"Chain":[]}}`
	require.Equal(t, expected, string(data))

	_, err = format.Encode(fake.NewBadContext(), prepare)
	require.EqualError(t, err, "couldn't serialize message: fake error")

	prepare = types.NewPrepare(fake.Message{}, fake.NewBadSignature(), types.NewChain())
	_, err = format.Encode(ctx, prepare)
	require.EqualError(t, err, "couldn't serialize signature: fake error")

	prepare = types.NewPrepare(fake.Message{}, fake.Signature{}, badChain{})
	_, err = format.Encode(ctx, prepare)
	require.EqualError(t, err, "couldn't serialize chain: oops")
}

func TestMessageFormat_Commit_Encode(t *testing.T) {
	commit := types.NewCommit([]byte{1}, fake.Signature{})

	format := requestFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, commit)
	require.NoError(t, err)
	require.Equal(t, `{"Commit":{"To":"AQ==","Prepare":{}}}`, string(data))

	_, err = format.Encode(fake.NewContext(), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), commit)
	require.EqualError(t, err, "couldn't marshal: fake error")

	commit = types.NewCommit([]byte{1}, fake.NewBadSignature())
	_, err = format.Encode(ctx, commit)
	require.EqualError(t, err, "couldn't serialize prepare: fake error")
}

func TestMessageFormat_Propagate_Encode(t *testing.T) {
	propagate := types.NewPropagate([]byte{1}, fake.Signature{})

	format := requestFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, propagate)
	require.NoError(t, err)
	require.Equal(t, `{"Propagate":{"To":"AQ==","Commit":{}}}`, string(data))

	propagate = types.NewPropagate([]byte{1}, fake.NewBadSignature())
	_, err = format.Encode(ctx, propagate)
	require.EqualError(t, err, "couldn't serialize commit: fake error")
}

func TestMessageFormat_Decode(t *testing.T) {
	format := requestFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)
	ctx = serde.WithFactory(ctx, types.MsgKeyFac{}, fake.MessageFactory{})
	ctx = serde.WithFactory(ctx, types.SigKeyFac{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.CoSigKeyFac{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.ChangeSetKeyFac{}, fakeChangeSetFactory{})
	ctx = serde.WithFactory(ctx, types.ChainKeyFac{}, types.ChainFactory{})

	request, err := format.Decode(ctx, []byte(`{"Prepare":{"Chain":[]}}`))
	require.NoError(t, err)
	require.IsType(t, types.Prepare{}, request)

	request, err = format.Decode(ctx, []byte(`{"Commit":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.Commit{}, request)

	request, err = format.Decode(ctx, []byte(`{"Propagate":{}}`))
	require.NoError(t, err)
	require.IsType(t, types.Propagate{}, request)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't unmarshal request: fake error")

	badCtx := serde.WithFactory(ctx, types.MsgKeyFac{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Prepare":{}}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx = serde.WithFactory(ctx, types.SigKeyFac{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{"Prepare":{}}`))
	require.EqualError(t, err, "couldn't deserialize signature: fake error")

	badCtx = serde.WithFactory(ctx, types.SigKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Prepare":{}}`))
	require.EqualError(t, err, "couldn't deserialize signature: invalid factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx, types.ChainKeyFac{}, badChainFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Prepare":{}}`))
	require.EqualError(t, err, "couldn't deserialize chain: oops")

	badCtx = serde.WithFactory(ctx, types.ChainKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Prepare":{}}`))
	require.EqualError(t, err, "couldn't deserialize chain: invalid factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx, types.CoSigKeyFac{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{"Commit":{}}`))
	require.EqualError(t, err, "couldn't deserialize commit: fake error")

	_, err = format.Decode(badCtx, []byte(`{"Propagate":{}}`))
	require.EqualError(t, err, "couldn't deserialize propagate: fake error")

	badCtx = serde.WithFactory(ctx, types.CoSigKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Commit":{}}`))
	require.EqualError(t, err, "couldn't deserialize commit: invalid factory of type '<nil>'")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeFL(t *testing.T, opts ...types.ForwardLinkOption) types.ForwardLink {
	opts = append([]types.ForwardLinkOption{
		types.WithPrepare(fake.Signature{}),
		types.WithCommit(fake.Signature{}),
		types.WithChangeSet(fakeChangeSet{}),
	}, opts...)

	fl, err := types.NewForwardLink([]byte{1}, []byte{2}, opts...)
	require.NoError(t, err)

	return fl
}

type fakeChangeSet struct {
	viewchange.ChangeSet
	err error
}

func (cs fakeChangeSet) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), cs.err
}

type fakeChangeSetFactory struct {
	viewchange.ChangeSetFactory
	err error
}

func (f fakeChangeSetFactory) ChangeSetOf(serde.Context, []byte) (viewchange.ChangeSet, error) {
	return fakeChangeSet{}, f.err
}

type badChain struct {
	consensus.Chain
}

func (c badChain) Serialize(serde.Context) ([]byte, error) {
	return nil, xerrors.New("oops")
}

type badChainFactory struct {
	consensus.ChainFactory
}

func (f badChainFactory) ChainOf(serde.Context, []byte) (consensus.Chain, error) {
	return nil, xerrors.New("oops")
}
