package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"golang.org/x/xerrors"
)

func init() {
	rand.Seed(0)
}

// Start 3 nodes
// Use the value contract
// Check the state
func TestIntegration_Value_Simple(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela-integration-test")
	require.NoError(t, err)

	timeout := time.Second * 10 // transaction inclusion timeout

	t.Logf("using temps dir %s", dir)

	defer os.RemoveAll(dir)

	nodes := []dela{
		newDelaNode(t, filepath.Join(dir, "node1"), 0),
		newDelaNode(t, filepath.Join(dir, "node2"), 0),
		newDelaNode(t, filepath.Join(dir, "node3"), 0),
	}

	nodes[0].Setup(nodes[1:]...)

	l := loader.NewFileLoader(filepath.Join(dir, "private.key"))

	signerdata, err := l.LoadOrCreate(newKeyGenerator())
	require.NoError(t, err)

	signer, err := bls.NewSignerFromBytes(signerdata)
	require.NoError(t, err)

	pubKey := signer.GetPublicKey()
	cred := accessContract.NewCreds(aKey[:])

	for _, node := range nodes {
		node.GetAccessService().Grant(node.(cosiDelaNode).GetAccessStore(), cred, pubKey)
	}

	manager := signed.NewManager(signer, &txClient{})

	pubKeyBuf, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)

	args := []txn.Arg{
		{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Access")},
		{Key: "access:grant_id", Value: []byte(hex.EncodeToString(valueAccessKey[:]))},
		{Key: "access:grant_contract", Value: []byte("go.dedis.ch/dela.Value")},
		{Key: "access:grant_command", Value: []byte("all")},
		{Key: "access:identity", Value: []byte(base64.StdEncoding.EncodeToString(pubKeyBuf))},
		{Key: "access:command", Value: []byte("GRANT")},
	}
	err = addAndWait(t, timeout, manager, nodes[0].(cosiDelaNode), args...)
	require.NoError(t, err)

	key1 := make([]byte, 32)

	_, err = rand.Read(key1)
	require.NoError(t, err)

	args = []txn.Arg{
		{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Value")},
		{Key: "value:key", Value: key1},
		{Key: "value:value", Value: []byte("value1")},
		{Key: "value:command", Value: []byte("WRITE")},
	}
	err = addAndWait(t, timeout, manager, nodes[0].(cosiDelaNode), args...)
	require.NoError(t, err)

	proof, err := nodes[0].GetOrdering().GetProof(key1)
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), proof.GetValue())

	key2 := make([]byte, 32)

	_, err = rand.Read(key2)
	require.NoError(t, err)

	args = []txn.Arg{
		{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Value")},
		{Key: "value:key", Value: key2},
		{Key: "value:value", Value: []byte("value2")},
		{Key: "value:command", Value: []byte("WRITE")},
	}
	err = addAndWait(t, timeout, manager, nodes[0].(cosiDelaNode), args...)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func addAndWait(t *testing.T, to time.Duration, manager txn.Manager, node cosiDelaNode, args ...txn.Arg) error {
	manager.Sync()

	tx, err := manager.Make(args...)
	require.NoError(t, err)

	err = node.GetPool().Add(tx)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	events := node.GetOrdering().Watch(ctx)

	for event := range events {
		for _, result := range event.Transactions {
			tx := result.GetTransaction()

			if bytes.Equal(tx.GetID(), tx.GetID()) {
				accepted, err := event.Transactions[0].GetStatus()
				require.Empty(t, err)

				require.True(t, accepted)
				return nil
			}
		}
	}

	return xerrors.Errorf("transaction not found")
}
