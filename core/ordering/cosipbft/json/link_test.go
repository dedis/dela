package json

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	types.RegisterBlockFormat(fake.GoodFormat, fakeBlockFormat{})
	types.RegisterBlockFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestLinkFormat_Encode(t *testing.T) {
	format := linkFormat{}

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, makeLink(t))
	require.NoError(t, err)
	re := `{"From":"[^"]+","To":"[^"]+",` +
		`"PrepareSignature":{},"CommitSignature":{},"ChangeSet":{}}`
	require.Regexp(t, re, string(data))

	data, err = format.Encode(ctx, makeBlockLink(t))
	require.NoError(t, err)
	re = `{"From":"[^"]+","PrepareSignature":{},` +
		`"CommitSignature":{},"ChangeSet":{},"Block":{}}`
	require.Regexp(t, re, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message 'fake.Message'")

	opt := types.WithSignatures(fake.NewBadSignature(), fake.Signature{})
	_, err = format.Encode(ctx, makeLink(t, opt))
	require.EqualError(t, err, fake.Err("couldn't serialize prepare"))

	opt = types.WithSignatures(fake.Signature{}, fake.NewBadSignature())
	_, err = format.Encode(ctx, makeLink(t, opt))
	require.EqualError(t, err, fake.Err("couldn't serialize commit"))

	opt = types.WithChangeSet(fakeChangeSet{err: fake.GetError()})
	_, err = format.Encode(ctx, makeBlockLink(t, opt))
	require.EqualError(t, err, fake.Err("couldn't serialize change set"))

	_, err = format.Encode(fake.NewBadContext(), makeBlockLink(t))
	require.EqualError(t, err, fake.Err("couldn't serialize block: encoding failed"))

	_, err = format.Encode(fake.NewBadContext(), makeLink(t))
	require.EqualError(t, err, fake.Err("failed to marshal"))
}

func TestLinkFormat_Decode(t *testing.T) {
	format := linkFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.AggregateKey{}, fake.SignatureFactory{})
	ctx = serde.WithFactory(ctx, types.ChangeSetKey{}, fakeChangeSetFac{})
	ctx = serde.WithFactory(ctx, types.BlockKey{}, types.BlockFactory{})

	msg, err := format.Decode(ctx, []byte(`{"From":[1],"To":[2]}`))
	require.NoError(t, err)
	require.Equal(t, makeLink(t), msg)

	msg, err = format.Decode(ctx, []byte(`{"From":[1],"Block":{}}`))
	require.NoError(t, err)
	require.Equal(t, makeBlockLink(t), msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	badCtx := serde.WithFactory(ctx, types.AggregateKey{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to decode prepare: factory failed"))

	badCtx = serde.WithFactory(ctx, types.AggregateKey{}, fake.NewBadSignatureFactoryWithDelay(1))
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to decode commit: factory failed"))

	badCtx = serde.WithFactory(ctx, types.ChangeSetKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "failed to decode change set: invalid factory 'fake.MessageFactory'")

	badCtx = serde.WithFactory(ctx, types.ChangeSetKey{}, fakeChangeSetFac{err: fake.GetError()})
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to decode change set: factory failed"))

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "missing block factory")

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, fake.Err("failed to decode block"))

	badCtx = serde.WithFactory(ctx, types.BlockKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Block":{}}`))
	require.EqualError(t, err, "invalid block 'fake.Message'")

	format.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating forward link: failed to fingerprint: ")

	_, err = format.Decode(ctx, []byte(`{"Block":{}}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating block link: creating forward link: failed to fingerprint: ")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeLink(t *testing.T, opts ...types.LinkOption) types.Link {
	sigs := types.WithSignatures(fake.Signature{}, fake.Signature{})
	cs := types.WithChangeSet(fakeChangeSet{})

	opts = append([]types.LinkOption{sigs, cs}, opts...)

	link, err := types.NewForwardLink(types.Digest{1}, types.Digest{2}, opts...)
	require.NoError(t, err)

	return link
}

func makeBlockLink(t *testing.T, opts ...types.LinkOption) types.BlockLink {
	block, err := types.NewBlock(fakeResult{})
	require.NoError(t, err)

	sigs := types.WithSignatures(fake.Signature{}, fake.Signature{})
	cs := types.WithChangeSet(fakeChangeSet{})

	opts = append([]types.LinkOption{sigs, cs}, opts...)

	link, err := types.NewBlockLink(types.Digest{1}, block, opts...)
	require.NoError(t, err)

	return link
}

type fakeChangeSet struct {
	authority.ChangeSet

	err error
}

func (cs fakeChangeSet) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), cs.err
}

type fakeChangeSetFac struct {
	authority.ChangeSetFactory

	err error
}

func (fac fakeChangeSetFac) ChangeSetOf(serde.Context, []byte) (authority.ChangeSet, error) {
	return fakeChangeSet{}, fac.err
}

type fakeResult struct {
	validation.Result

	err error
}

func (data fakeResult) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), data.err
}

func (fakeResult) Fingerprint(io.Writer) error {
	return nil
}

type fakeResultFac struct {
	validation.ResultFactory

	err error
}

func (fac fakeResultFac) ResultOf(serde.Context, []byte) (validation.Result, error) {
	return fakeResult{}, fac.err
}

type fakeBlockFormat struct {
	serde.FormatEngine
}

func (fakeBlockFormat) Encode(serde.Context, serde.Message) ([]byte, error) {
	return []byte(`{}`), nil
}

func (fakeBlockFormat) Decode(serde.Context, []byte) (serde.Message, error) {
	block, err := types.NewBlock(fakeResult{})
	if err != nil {
		return nil, err
	}

	return block, nil
}
