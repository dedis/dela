package types

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterGenesisFormat(fake.GoodFormat, fake.Format{Msg: Genesis{}})
	RegisterGenesisFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterBlockFormat(fake.GoodFormat, fake.Format{Msg: Block{}})
	RegisterBlockFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestDigest_String(t *testing.T) {
	digest := Digest{1, 2, 3, 4}

	require.Equal(t, "01020304", digest.String())
}

func TestDigest_Bytes(t *testing.T) {
	digest := Digest{1, 2, 3, 4}

	require.Equal(t, digest[:], digest.Bytes())
}

func TestGenesis_GetHash(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := NewGenesis(ro)
	require.NoError(t, err)

	require.NotEqual(t, Digest{}, genesis.GetHash())

	id := Digest{1, 2, 3}
	genesis.digest = id
	require.Equal(t, id, genesis.GetHash())
}

func TestGenesis_GetRoster(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := NewGenesis(ro)
	require.NoError(t, err)

	require.Equal(t, 3, genesis.GetRoster().Len())
}

func TestGenesis_GetRoot(t *testing.T) {
	genesis := Genesis{treeRoot: Digest{5}}

	require.Equal(t, Digest{5}, genesis.GetRoot())
}

func TestGenesis_Serialize(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := NewGenesis(ro)
	require.NoError(t, err)

	data, err := genesis.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = genesis.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestGenesis_Fingerprint(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(1, fake.NewSigner))

	genesis, err := NewGenesis(ro, WithGenesisRoot(Digest{5}))
	require.NoError(t, err)

	buffer := new(bytes.Buffer)
	err = genesis.Fingerprint(buffer)
	require.NoError(t, err)
	require.Regexp(t, "^\x05(\x00){35,}PK", buffer.String())

	_, err = NewGenesis(ro, WithGenesisHashFactory(fake.NewHashFactory(fake.NewBadHash())))
	require.EqualError(t, err, fake.Err("fingerprint failed: couldn't write root"))

	genesis.roster = badRoster{}
	err = genesis.Fingerprint(buffer)
	require.EqualError(t, err, fake.Err("roster fingerprint failed"))
}

func TestGenesisFactory_Deserialize(t *testing.T) {
	fac := NewGenesisFactory(authority.NewFactory(nil, nil))

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, Genesis{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding failed"))
}

func TestBlock_GetHash(t *testing.T) {
	block, err := NewBlock(simple.NewResult(nil), WithTreeRoot(Digest{2}))
	require.NoError(t, err)
	require.NotEqual(t, Digest{}, block.GetHash())
}

func TestBlock_GetIndex(t *testing.T) {
	block, err := NewBlock(simple.NewResult(nil), WithIndex(2))
	require.NoError(t, err)
	require.Equal(t, uint64(2), block.GetIndex())
}

func TestBlock_GetData(t *testing.T) {
	block := Block{data: simple.NewResult(nil)}

	require.Equal(t, simple.NewResult(nil), block.GetData())
}

func TestBlock_GetTransactions(t *testing.T) {
	block := Block{data: simple.NewResult(nil)}
	require.Len(t, block.GetTransactions(), 0)

	block.data = simple.NewResult([]simple.TransactionResult{{}})
	require.Len(t, block.GetTransactions(), 1)
}

func TestBlock_GetTreeRoot(t *testing.T) {
	block := Block{treeRoot: Digest{3}}

	require.Equal(t, Digest{3}, block.GetTreeRoot())
}

func TestBlock_Fingerprint(t *testing.T) {
	block := Block{
		index:    3,
		treeRoot: Digest{4},
		data:     simple.NewResult(nil),
	}

	buffer := new(bytes.Buffer)

	err := block.Fingerprint(buffer)
	require.NoError(t, err)
	require.Regexp(t, "^\x03(\x00){7}\x04(\x00){31}$", buffer.String())

	err = block.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, fake.Err("couldn't write index"))

	err = block.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, fake.Err("couldn't write root"))

	block.data = badData{}
	err = block.Fingerprint(io.Discard)
	require.EqualError(t, err, fake.Err("data fingerprint failed"))

	_, err = NewBlock(block.data, WithHashFactory(fake.NewHashFactory(fake.NewBadHash())))
	require.EqualError(t, err, fake.Err("fingerprint failed: couldn't write index"))
}

func TestBlock_Serialize(t *testing.T) {
	block, err := NewBlock(simple.NewResult(nil))
	require.NoError(t, err)

	data, err := block.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = block.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestBlockFactory_Deserialize(t *testing.T) {
	txFac := signed.NewTransactionFactory()
	fac := NewBlockFactory(simple.NewResultFactory(txFac))

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, Block{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding block failed"))
}

// -----------------------------------------------------------------------------
// Utility functions

type badRoster struct {
	authority.Authority
}

func (r badRoster) Fingerprint(io.Writer) error {
	return fake.GetError()
}

type badData struct {
	validation.Result
}

func (d badData) Fingerprint(io.Writer) error {
	return fake.GetError()
}
