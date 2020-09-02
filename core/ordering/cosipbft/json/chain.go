package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

type chainFormat struct{}

func (fmt chainFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	chain, ok := msg.(types.Chain)
	if !ok {
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	links := chain.GetLinks()
	raws := make([]json.RawMessage, len(links))

	for i, link := range links {
		raw, err := link.Serialize(ctx)
		if err != nil {
			return nil, err
		}

		raws[i] = raw
	}

	m := ChainJSON{
		Links: raws,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (fmt chainFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := ChainJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	if len(m.Links) == 0 {
		return nil, xerrors.New("chain cannot be empty")
	}

	fac, ok := ctx.GetFactory(types.LinkKey{}).(types.LinkFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory")
	}

	prevs := make([]types.Link, len(m.Links)-1)
	for i, raw := range m.Links[:len(m.Links)-1] {
		link, err := fac.LinkOf(ctx, raw)
		if err != nil {
			return nil, err
		}

		prevs[i] = link
	}

	last, err := fac.BlockLinkOf(ctx, m.Links[len(m.Links)-1])
	if err != nil {
		return nil, err
	}

	return types.NewChain(last, prevs), nil
}
