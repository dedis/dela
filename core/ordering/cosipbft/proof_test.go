package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestProof_GetKey(t *testing.T) {
	p := Proof{
		path: fakePath{},
	}

	require.Equal(t, []byte("key"), p.GetKey())
}

func TestProof_GetValue(t *testing.T) {
	p := Proof{
		path: fakePath{},
	}

	require.Equal(t, []byte("value"), p.GetValue())
}

func TestProof_Verify(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := types.NewGenesis(ro)
	require.NoError(t, err)

	block, err := types.NewBlock(simple.NewResult(nil))
	require.NoError(t, err)

	p := Proof{
		path:  fakePath{},
		chain: fakeChain{block: block},
	}

	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "mismatch tree root: '00000000' != '01020300'")

	p.chain = fakeChain{err: fake.GetError()}
	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, fake.Err("failed to verify chain"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePath struct {
	hashtree.Path
}

func (p fakePath) GetKey() []byte {
	return []byte("key")
}

func (p fakePath) GetValue() []byte {
	return []byte("value")
}

func (p fakePath) GetRoot() []byte {
	return types.Digest{1, 2, 3}.Bytes()
}

type fakeChain struct {
	types.Chain

	block types.Block
	err   error
}

func (c fakeChain) GetBlock() types.Block {
	return c.block
}

func (c fakeChain) Verify(types.Genesis, crypto.VerifierFactory) error {
	return c.err
}
