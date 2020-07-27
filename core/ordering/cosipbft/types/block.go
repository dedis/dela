package types

import (
	"io"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

var (
	genesisFormats = registry.NewSimpleRegistry()
	blockFormats   = registry.NewSimpleRegistry()
)

// RegisterGenesisFormat registers the engine for the provided format.
func RegisterGenesisFormat(f serde.Format, e serde.FormatEngine) {
	genesisFormats.Register(f, e)
}

// RegisterBlockFormat registers the engine for the provided format.
func RegisterBlockFormat(f serde.Format, e serde.FormatEngine) {
	blockFormats.Register(f, e)
}

// Genesis is the very first block of a chain. It contains the initial roster
// and tree root.
type Genesis struct {
	digest   []byte
	roster   viewchange.Authority
	treeRoot []byte
}

type genesisTemplate struct {
	Genesis
	hashFactory crypto.HashFactory
}

// GenesisOption is the option type to set some fields of a genesis block.
type GenesisOption func(*genesisTemplate)

// NewGenesis creates a new genesis block with the provided roster.
func NewGenesis(ca crypto.CollectiveAuthority, opts ...GenesisOption) (Genesis, error) {
	tmpl := genesisTemplate{
		Genesis: Genesis{
			roster:   roster.FromAuthority(ca),
			treeRoot: []byte{},
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return tmpl.Genesis, err
	}

	tmpl.digest = h.Sum(nil)

	return tmpl.Genesis, nil
}

func (g Genesis) GetHash() []byte {
	return append([]byte{}, g.digest...)
}

// GetRoster returns the roster of the genesis block.
func (g Genesis) GetRoster() viewchange.Authority {
	return g.roster
}

// GetRoot returns the tree root.
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

// Fingerprint implements serde.Fingerprinter.
func (g Genesis) Fingerprint(w io.Writer) error {
	_, err := w.Write(g.treeRoot)
	if err != nil {
		return err
	}

	err = g.roster.Fingerprint(w)
	if err != nil {
		return err
	}

	return nil
}

// RosterKey is the key of the roster factory.
type RosterKey struct{}

// GenesisFactory is a factory to deserialize the genesis messages.
type GenesisFactory struct {
	rosterFac viewchange.AuthorityFactory
}

// NewGenesisFactory creates a new genesis factory.
func NewGenesisFactory(rf viewchange.AuthorityFactory) GenesisFactory {
	return GenesisFactory{
		rosterFac: rf,
	}
}

// Deserialize implements serde.Factory.
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
type BlockLink struct {
	from       []byte
	to         Block
	changeset  viewchange.ChangeSet
	prepareSig crypto.Signature
	commitSig  crypto.Signature
}

func NewForwardLink(from []byte, to Block) BlockLink {
	return BlockLink{
		from: from,
		to:   to,
	}
}

func (link BlockLink) GetFrom() []byte {
	return append([]byte{}, link.from...)
}

func (link BlockLink) GetTo() Block {
	return link.to
}

func (link BlockLink) Fingerprint(w io.Writer) error {
	w.Write(link.from)
	w.Write(link.to.GetHash())
	return nil
}

// Block is a block of the chain.
type Block struct {
	digest   []byte
	data     validation.Data
	treeRoot []byte
}

type blockTemplate struct {
	Block
	hashFactory crypto.HashFactory
}

type BlockOption func(*blockTemplate)

func WithTreeRoot(root []byte) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.treeRoot = root
	}
}

// NewBlock creates a new block.
func NewBlock(data validation.Data, opts ...BlockOption) (Block, error) {
	tmpl := blockTemplate{
		Block: Block{
			data:     data,
			treeRoot: []byte{},
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return tmpl.Block, err
	}

	tmpl.digest = h.Sum(nil)

	return tmpl.Block, nil
}

func (b Block) GetHash() []byte {
	return append([]byte{}, b.digest...)
}

func (b Block) GetData() validation.Data {
	return b.data
}

func (b Block) GetTransactions() []tap.Transaction {
	results := b.data.GetTransactionResults()
	txs := make([]tap.Transaction, len(results))

	for i, res := range results {
		txs[i] = res.GetTransaction()
	}

	return txs
}

func (b Block) GetTreeRoot() []byte {
	return b.treeRoot
}

func (b Block) Fingerprint(w io.Writer) error {
	_, err := w.Write(b.treeRoot)
	if err != nil {
		return err
	}

	err = b.data.Fingerprint(w)
	if err != nil {
		return err
	}

	return nil
}

// Serialize implements serde.Message.
func (b Block) Serialize(ctx serde.Context) ([]byte, error) {
	format := blockFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type DataKey struct{}

type BlockFactory struct {
	dataFac validation.DataFactory
}

func NewBlockFactory(fac validation.DataFactory) BlockFactory {
	return BlockFactory{
		dataFac: fac,
	}
}

func (f BlockFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := blockFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, DataKey{}, f.dataFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
