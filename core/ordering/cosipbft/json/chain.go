package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// ChainFormat is the JSON format to encode and decode chains.
//
// - implements serde.FormatEngine
type chainFormat struct{}

// Encode implements serde.FormatEngine. It serializes the chain if appropriate,
// otherwise it returns an error.
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
			return nil, xerrors.Errorf("couldn't serialize link: %v", err)
		}

		raws[i] = raw
	}

	m := ChainJSON{
		Links: raws,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It deserializes the chain if
// appropriate, otherwise it returns an error.
func (fmt chainFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := ChainJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	if len(m.Links) == 0 {
		return nil, xerrors.New("chain cannot be empty")
	}

	fac := ctx.GetFactory(types.LinkKey{})

	factory, ok := fac.(types.LinkFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid link factory '%T'", fac)
	}

	prevs := make([]types.Link, len(m.Links)-1)
	for i, raw := range m.Links[:len(m.Links)-1] {
		link, err := factory.LinkOf(ctx, raw)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize link: %v", err)
		}

		prevs[i] = link
	}

	last, err := factory.BlockLinkOf(ctx, m.Links[len(m.Links)-1])
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize block link: %v", err)
	}

	return types.NewChain(last, prevs), nil
}
