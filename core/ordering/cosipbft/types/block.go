package types

import (
	"encoding/binary"
	"fmt"
	"io"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
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

// Digest defines the result of a fingerprint. It expects a digest of 256 bits.
type Digest [32]byte

func (d Digest) String() string {
	return fmt.Sprintf("%x", d[:])[:8]
}

// Genesis is the very first block of a chain. It contains the initial roster
// and tree root.
type Genesis struct {
	digest   Digest
	roster   viewchange.Authority
	treeRoot Digest
}

type genesisTemplate struct {
	Genesis
	hashFactory crypto.HashFactory
}

// GenesisOption is the option type to set some fields of a genesis block.
type GenesisOption func(*genesisTemplate)

// WithGenesisRoot is an option to set the tree root of the genesis block.
func WithGenesisRoot(root Digest) GenesisOption {
	return func(tmpl *genesisTemplate) {
		tmpl.treeRoot = root
	}
}

// WithHashFactory is an option to set the hash factory.
func WithHashFactory(fac crypto.HashFactory) GenesisOption {
	return func(tmpl *genesisTemplate) {
		tmpl.hashFactory = fac
	}
}

// NewGenesis creates a new genesis block with the provided roster.
func NewGenesis(ca crypto.CollectiveAuthority, opts ...GenesisOption) (Genesis, error) {
	tmpl := genesisTemplate{
		Genesis: Genesis{
			roster:   roster.FromAuthority(ca),
			treeRoot: Digest{},
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return tmpl.Genesis, xerrors.Errorf("fingerprint failed: %v", err)
	}

	copy(tmpl.digest[:], h.Sum(nil))

	return tmpl.Genesis, nil
}

// GetHash returns the digest of the block.
func (g Genesis) GetHash() Digest {
	return g.digest
}

// GetRoster returns the roster of the genesis block.
func (g Genesis) GetRoster() viewchange.Authority {
	return g.roster
}

// GetRoot returns the tree root.
func (g Genesis) GetRoot() Digest {
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
	_, err := w.Write(g.treeRoot[:])
	if err != nil {
		return xerrors.Errorf("couldn't write root: %v", err)
	}

	err = g.roster.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("roster fingerprint failed: %v", err)
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

// BlockLink contains the different proofs that a block has been committed by
// the collective authority.
type BlockLink struct {
	from       Digest
	to         Block
	changeset  viewchange.ChangeSet
	prepareSig crypto.Signature
	commitSig  crypto.Signature
}

// NewBlockLink creates a new block link between from and to.
func NewBlockLink(from Digest, to Block) BlockLink {
	return BlockLink{
		from: from,
		to:   to,
	}
}

// GetFrom returns the digest of the source block.
func (link BlockLink) GetFrom() Digest {
	return link.from
}

// GetTo returns the block the link is pointing to.
func (link BlockLink) GetTo() Block {
	return link.to
}

// GetPrepareSignature returns the prepare signature if it is set, otherwise it
// returns nil.
func (link BlockLink) GetPrepareSignature() crypto.Signature {
	return link.prepareSig
}

// GetCommitSignature returns the commit signature if it is set, otherwise it
// returns nil.
func (link BlockLink) GetCommitSignature() crypto.Signature {
	return link.commitSig
}

// GetChangeSet returns the change set of the roster for this link.
func (link BlockLink) GetChangeSet() viewchange.ChangeSet {
	return link.changeset
}

// Fingerprint implements serde.Fingerprinter. It deterministically writes a
// binary representation of the block link.
func (link BlockLink) Fingerprint(w io.Writer) error {
	_, err := w.Write(link.from[:])
	if err != nil {
		return xerrors.Errorf("couldn't write from: %v", err)
	}

	id := link.GetTo().GetHash()

	_, err = w.Write(id[:])
	if err != nil {
		return xerrors.Errorf("couldn't write to: %v", err)
	}

	return nil
}

// Block is a block of the chain.
type Block struct {
	digest   Digest
	index    uint64
	data     validation.Data
	treeRoot Digest
}

type blockTemplate struct {
	Block
	hashFactory crypto.HashFactory
}

// BlockOption is the type of option to set some fields of a block.
type BlockOption func(*blockTemplate)

// WithIndex is an option to set the index of the block.
func WithIndex(index uint64) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.index = index
	}
}

// WithTreeRoot is an option to set the tree root for the block.
func WithTreeRoot(root Digest) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.treeRoot = root
	}
}

// NewBlock creates a new block.
func NewBlock(data validation.Data, opts ...BlockOption) (Block, error) {
	tmpl := blockTemplate{
		Block: Block{
			data:     data,
			treeRoot: Digest{},
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

	copy(tmpl.digest[:], h.Sum(nil))

	return tmpl.Block, nil
}

// GetHash returns the digest of the block.
func (b Block) GetHash() Digest {
	return b.digest
}

// GetData returns the validated data of the block.
func (b Block) GetData() validation.Data {
	return b.data
}

// GetTransactions is a helper to extract the transactions from the validated
// data.
func (b Block) GetTransactions() []tap.Transaction {
	results := b.data.GetTransactionResults()
	txs := make([]tap.Transaction, len(results))

	for i, res := range results {
		txs[i] = res.GetTransaction()
	}

	return txs
}

// GetTreeRoot returns the tree root of the block.
func (b Block) GetTreeRoot() Digest {
	return b.treeRoot
}

// Fingerprint implements serde.Fingerprinter. It deterministically writes a
// binary representation of the block into the writer.
func (b Block) Fingerprint(w io.Writer) error {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, b.index)
	_, err := w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write index: %v", err)
	}

	_, err = w.Write(b.treeRoot[:])
	if err != nil {
		return xerrors.Errorf("couldn't write root: %v", err)
	}

	err = b.data.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("data fingerprint failed: %v", err)
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

// DataKey is the key for the validated data factory.
type DataKey struct{}

// BlockFactory is a factory to deserialize block messages.
type BlockFactory struct {
	dataFac validation.DataFactory
}

// NewBlockFactory creates a new block factory.
func NewBlockFactory(fac validation.DataFactory) BlockFactory {
	return BlockFactory{
		dataFac: fac,
	}
}

// Deserialize implements serde.Factory. It populates the block from the data if
// appropriate, otherwise it returns an error.
func (f BlockFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := blockFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, DataKey{}, f.dataFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
