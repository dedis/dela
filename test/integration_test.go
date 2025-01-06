package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/contracts/value"
	"go.dedis.ch/dela/core/store/prefixed"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"golang.org/x/xerrors"
)

var randGen *rand.Rand

func init() {
	randGen = rand.New(rand.NewSource(0))
}

// Start 3 nodes
// Use the value contract
// Check the state
func TestIntegration_Value_Simple(t *testing.T) {
	t.Run("3 nodes: ws", getTest[*testing.T](3, 2, minoWS))
}

func BenchmarkValue(b *testing.B) {
	testWS := func(b *testing.B) {
		getTest[*testing.B](5, b.N, minoWS)(b)
	}
	b.Run("5 nodes: ws", testWS)
}

func getTest[T require.TestingT](numNode, numTx int, kind string) func(t T) {
	return func(t T) {
		dir, err := os.MkdirTemp(os.TempDir(), "dela-integration-test")
		require.NoError(t, err)

		timeout := time.Second * 10 // transaction inclusion timeout

		defer os.RemoveAll(dir)

		nodes := make([]dela, numNode)

		for i := range nodes {
			node := newDelaNode(t, filepath.Join(dir,
				"node"+strconv.Itoa(i)), 0, kind)
			nodes[i] = node
		}

		nodes[0].Setup(kind, nodes[1:]...)

		l := loader.NewFileLoader(filepath.Join(dir, "private.key"))

		signerdata, err := l.LoadOrCreate(newKeyGenerator())
		require.NoError(t, err)

		signer, err := bls.NewSignerFromBytes(signerdata)
		require.NoError(t, err)

		pubKey := signer.GetPublicKey()
		cred := accessContract.NewCreds()

		for _, node := range nodes {
			node.GetAccessService().Grant(node.(cosiDelaNode).GetAccessStore(), cred, pubKey)
		}

		manager := signed.NewManager(signer, &txClient{})

		pubKeyBuf, err := signer.GetPublicKey().MarshalBinary()
		require.NoError(t, err)

		args := []txn.Arg{
			{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Access")},
			{Key: "access:grant_id", Value: []byte(hex.EncodeToString([]byte(value.ContractUID)))},
			{Key: "access:grant_contract", Value: []byte("go.dedis.ch/dela.Value")},
			{Key: "access:grant_command", Value: []byte("all")},
			{Key: "access:identity", Value: []byte(base64.StdEncoding.EncodeToString(pubKeyBuf))},
			{Key: "access:command", Value: []byte("GRANT")},
		}

		err = addAndWait(t, timeout, manager, nodes[0].(cosiDelaNode), args...)
		require.NoError(t, err)

		for i := 0; i < numTx; i++ {
			key := make([]byte, 32)

			_, err = randGen.Read(key)
			require.NoError(t, err)

			args = []txn.Arg{
				{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Value")},
				{Key: "value:key", Value: key},
				{Key: "value:value", Value: []byte("value1")},
				{Key: "value:command", Value: []byte("WRITE")},
			}

			err = addAndWait(t, timeout, manager, nodes[0].(cosiDelaNode), args...)
			require.NoError(t, err)

			prefixedKey := prefixed.NewPrefixedKey([]byte(value.ContractUID), key)
			proof, err := nodes[0].GetOrdering().GetProof(prefixedKey)
			require.NoError(t, err)
			require.Equal(t, []byte("value1"), proof.GetValue())
		}
	}
}

// -----------------------------------------------------------------------------
// Utility functions

func addAndWait(
	t require.TestingT,
	to time.Duration,
	manager txn.Manager,
	node cosiDelaNode,
	args ...txn.Arg,
) error {
	tx, err := manager.Make(args...)
	if err != nil {
		return xerrors.Errorf("failed to make tx: %v", err)
	}

	err = node.GetPool().Add(tx)
	if err != nil {
		return xerrors.Errorf("failed to add tx: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	events := node.GetOrdering().Watch(ctx)

	for event := range events {
		for _, result := range event.Transactions {
			txDone := result.GetTransaction()

			if bytes.Equal(tx.GetID(), txDone.GetID()) {
				accepted, err := event.Transactions[0].GetStatus()
				require.Empty(t, err)

				require.True(t, accepted)
				return nil
			}
		}
	}

	return xerrors.Errorf("transaction not found")
}
