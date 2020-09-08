package json

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// LinkFormat is the JSON format engine to serialize and deserialize the links.
//
// - implements serde.FormatEngine
type linkFormat struct {
	hashFac crypto.HashFactory
}

// Encode implements serde.FormatEngine. It serializes the link or the block
// link if appropriate, otherwise it returns an error.
func (fmt linkFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m LinkJSON

	switch link := msg.(type) {
	case types.BlockLink:
		err := fmt.encodeLink(ctx, link, &m)
		if err != nil {
			return nil, err
		}

		block, err := link.GetBlock().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize block: %v", err)
		}

		m.Block = block
	case types.Link:
		err := fmt.encodeLink(ctx, link, &m)
		if err != nil {
			return nil, err
		}

		to := link.GetTo()

		m.To = to.Bytes()
	default:
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

func (fmt linkFormat) encodeLink(ctx serde.Context, link types.Link, m *LinkJSON) error {
	prepare, err := link.GetPrepareSignature().Serialize(ctx)
	if err != nil {
		return xerrors.Errorf("couldn't serialize prepare: %v", err)
	}

	commit, err := link.GetCommitSignature().Serialize(ctx)
	if err != nil {
		return xerrors.Errorf("couldn't serialize commit: %v", err)
	}

	changeset, err := link.GetChangeSet().Serialize(ctx)
	if err != nil {
		return xerrors.Errorf("couldn't serialize change set: %v", err)
	}

	m.From = link.GetFrom().Bytes()
	m.PrepareSignature = prepare
	m.CommitSignature = commit
	m.ChangeSet = changeset

	return nil
}

// Decode implements serde.FormatEngine. It populates the link or the block link
// if appropriate, otherwise it returns an error.
func (fmt linkFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := LinkJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	prepare, err := decodeSignature(ctx, m.PrepareSignature, types.AggregateKey{})
	if err != nil {
		return nil, xerrors.Errorf("failed to decode prepare: %v", err)
	}

	commit, err := decodeSignature(ctx, m.CommitSignature, types.AggregateKey{})
	if err != nil {
		return nil, xerrors.Errorf("failed to decode commit: %v", err)
	}

	changeset, err := decodeChangeSet(ctx, m.ChangeSet)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode change set: %v", err)
	}

	from := types.Digest{}
	copy(from[:], m.From)

	opts := []types.LinkOption{
		types.WithSignatures(prepare, commit),
		types.WithChangeSet(changeset),
	}

	if fmt.hashFac != nil {
		opts = append(opts, types.WithLinkHashFactory(fmt.hashFac))
	}

	if len(m.Block) > 0 {
		factory := ctx.GetFactory(types.BlockKey{})
		if factory == nil {
			return nil, xerrors.New("missing block factory")
		}

		msg, err := factory.Deserialize(ctx, m.Block)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode block: %v", err)
		}

		block, ok := msg.(types.Block)
		if !ok {
			return nil, xerrors.Errorf("invalid block '%T'", msg)
		}

		link, err := types.NewBlockLink(from, block, opts...)
		if err != nil {
			return nil, xerrors.Errorf("creating block link: %v", err)
		}

		return link, nil
	}

	to := types.Digest{}
	copy(to[:], m.To)

	link, err := types.NewForwardLink(from, to, opts...)
	if err != nil {
		return nil, xerrors.Errorf("creating forward link: %v", err)
	}

	return link, nil
}

func decodeChangeSet(ctx serde.Context, data []byte) (authority.ChangeSet, error) {
	factory := ctx.GetFactory(types.ChangeSetKey{})

	fac, ok := factory.(authority.ChangeSetFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory '%T'", factory)
	}

	changeset, err := fac.ChangeSetOf(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("factory failed: %v", err)
	}

	return changeset, nil
}
