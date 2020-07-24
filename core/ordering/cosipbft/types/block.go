package types

import (
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

var genesisFormats = registry.NewSimpleRegistry()

func RegisterGenesisFormat(f serde.Format, e serde.FormatEngine) {
	genesisFormats.Register(f, e)
}

// Genesis is the very first block of a chain. It contains the initial roster
// and tree root.
type Genesis struct {
	roster   viewchange.Authority
	treeRoot []byte
}

func NewGenesis(ca crypto.CollectiveAuthority) Genesis {
	return Genesis{
		roster: roster.FromAuthority(ca),
	}
}

func (g Genesis) GetRoster() viewchange.Authority {
	return g.roster
}

func (g Genesis) GetRoot() []byte {
	return g.treeRoot
}

// Serialize implements serde.Message.
func (g Genesis) Serialize(ctx serde.Context) ([]byte, error) {
	format := genesisFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, g)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type RosterKey struct{}

type GenesisFactory struct {
	rosterFac viewchange.AuthorityFactory
}

func NewGenesisFactory(rf viewchange.AuthorityFactory) GenesisFactory {
	return GenesisFactory{
		rosterFac: rf,
	}
}

func (f GenesisFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := genesisFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, RosterKey{}, f.rosterFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// ForwardLink contains the different proofs that a block has been committed by
// the collective authority.
type ForwardLink struct{}

// Block is a block of the chain.
type Block struct {
	data     validation.Data
	treeRoot []byte
	link     *ForwardLink
}

func NewBlock(data validation.Data, root []byte) Block {
	return Block{
		data:     data,
		treeRoot: root,
	}
}

// Serialize implements serde.Message.
func (b Block) Serialize(ctx serde.Context) ([]byte, error) {
	return []byte{}, nil
}
