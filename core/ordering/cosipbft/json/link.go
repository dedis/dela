package json

import (
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

type linkFormat struct{}

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
			return nil, err
		}

		m.Block = block
	case types.Link:
		err := fmt.encodeLink(ctx, link, &m)
		if err != nil {
			return nil, err
		}

		to := link.GetTo()

		m.To = &to
	default:
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (fmt linkFormat) encodeLink(ctx serde.Context, link types.Link, m *LinkJSON) error {
	prepare, err := link.GetPrepareSignature().Serialize(ctx)
	if err != nil {
		return err
	}

	commit, err := link.GetCommitSignature().Serialize(ctx)
	if err != nil {
		return err
	}

	changeset, err := link.GetChangeSet().Serialize(ctx)
	if err != nil {
		return err
	}

	m.From = link.GetFrom()
	m.PrepareSignature = prepare
	m.CommitSignature = commit
	m.ChangeSet = changeset

	return nil
}

func (fmt linkFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := LinkJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	prepare, err := decodeSignature(ctx, m.PrepareSignature)
	if err != nil {
		return nil, err
	}

	commit, err := decodeSignature(ctx, m.CommitSignature)
	if err != nil {
		return nil, err
	}

	changeset, err := decodeChangeSet(ctx, m.ChangeSet)
	if err != nil {
		return nil, err
	}

	opts := []types.LinkOption{
		types.WithSignatures(prepare, commit),
		types.WithChangeSet(changeset),
	}

	if len(m.Block) > 0 {
		factory := ctx.GetFactory(types.BlockKey{})

		msg, err := factory.Deserialize(ctx, m.Block)
		if err != nil {
			return nil, err
		}

		block, ok := msg.(types.Block)
		if !ok {
			return nil, xerrors.Errorf("invalid block '%T'", msg)
		}

		link, err := types.NewBlockLink(m.From, block, opts...)
		if err != nil {
			return nil, err
		}

		return link, nil
	}

	link, err := types.NewForwardLink(m.From, *m.To, opts...)
	if err != nil {
		return nil, err
	}

	return link, nil
}

func decodeChangeSet(ctx serde.Context, data []byte) (viewchange.ChangeSet, error) {
	factory := ctx.GetFactory(types.ChangeSetKey{})

	fac, ok := factory.(viewchange.ChangeSetFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid change set factory '%T'", factory)
	}

	changeset, err := fac.ChangeSetOf(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("change set decoding failed: %v", err)
	}

	return changeset, nil
}
