package cosipbft

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/validation/simple"
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
	genesis, err := types.NewGenesis(fake.NewAuthority(3, fake.NewSigner))
	require.NoError(t, err)

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(types.Digest{1, 2, 3}))
	require.NoError(t, err)

	opts := []types.BlockLinkOption{
		types.WithSignatures(fake.Signature{}, fake.Signature{}),
	}

	link, err := types.NewBlockLink(genesis.GetHash(), block, opts...)
	require.NoError(t, err)

	p := Proof{
		path:  fakePath{},
		chain: []types.BlockLink{link},
	}

	err = p.Verify(genesis, fake.VerifierFactory{})
	require.NoError(t, err)

	p.chain = nil
	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "chain is empty but at least one link is expected")

	p.chain = []types.BlockLink{makeBlock(t, types.Digest{})}
	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, fmt.Sprintf("mismatch from: '00000000' != '%v'", genesis.GetHash()))

	p.chain = []types.BlockLink{link}
	err = p.Verify(genesis, fake.NewBadVerifierFactory())
	require.EqualError(t, err, "verifier factory failed: fake error")

	p.chain = []types.BlockLink{makeBlock(t, genesis.GetHash())}
	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "unexpected nil signature in link")

	p.chain = []types.BlockLink{link}
	err = p.Verify(genesis, fake.NewVerifierFactory(fake.NewBadVerifier()))
	require.EqualError(t, err, "invalid prepare signature: fake error")

	p.chain = []types.BlockLink{makeBlock(t, genesis.GetHash(),
		types.WithSignatures(fake.NewBadSignature(), fake.Signature{}))}
	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "failed to marshal signature: fake error")

	p.chain = []types.BlockLink{link}
	err = p.Verify(genesis, fake.NewVerifierFactory(fake.NewBadVerifierWithDelay(1)))
	require.EqualError(t, err, "invalid commit signature: fake error")

	block, err = types.NewBlock(simple.NewData(nil))
	require.NoError(t, err)

	link, err = types.NewBlockLink(genesis.GetHash(), block, opts...)
	require.NoError(t, err)
	p.chain = []types.BlockLink{link}
	err = p.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "mismatch tree root: '00000000' != '01020300'")
}

// Utility functions -----------------------------------------------------------

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
