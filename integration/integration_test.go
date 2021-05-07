package integration

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

const certKeyName = "cert.key"
const privateKeyFile = "private.key"

var aKey = [32]byte{1}
var valueAccessKey = [32]byte{2}

func init() {
	rand.Seed(0)
}

// Start 3 nodes
// Use the value contract
// Check the state
func TestIntegration_Value_Simple(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-integration-test")
	require.NoError(t, err)

	t.Logf("using temps dir %s", dir)

	defer os.RemoveAll(dir)

	nodes := []dela{
		newDelaNode(t, filepath.Join(dir, "node1"), 2001),
		newDelaNode(t, filepath.Join(dir, "node2"), 2002),
		newDelaNode(t, filepath.Join(dir, "node3"), 2003),
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
	addAndWait(t, manager, nodes[0].(cosiDelaNode), args...)

	key1 := make([]byte, 32)

	_, err = rand.Read(key1)
	require.NoError(t, err)

	args = []txn.Arg{
		{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Value")},
		{Key: "value:key", Value: key1},
		{Key: "value:value", Value: []byte("value1")},
		{Key: "value:command", Value: []byte("WRITE")},
	}
	addAndWait(t, manager, nodes[0].(cosiDelaNode), args...)

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
	addAndWait(t, manager, nodes[0].(cosiDelaNode), args...)
}

// -----------------------------------------------------------------------------
// Utility functions

func addAndWait(t *testing.T, manager txn.Manager, node cosiDelaNode, args ...txn.Arg) {
	manager.Sync()

	tx, err := manager.Make(args...)
	require.NoError(t, err)

	err = node.GetPool().Add(tx)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	events := node.GetOrdering().Watch(ctx)

	for event := range events {
		for _, result := range event.Transactions {
			tx := result.GetTransaction()

			if bytes.Compare(tx.GetID(), tx.GetID()) == 0 {
				accepted, err := event.Transactions[0].GetStatus()
				require.Empty(t, err)

				require.True(t, accepted)
				return
			}
		}
	}

	t.Error("transaction not found")
}

// dela defines the common interface for a Dela node.
type dela interface {
	Setup(...dela)
	GetMino() mino.Mino
	GetOrdering() ordering.Service
	GetTxManager() txn.Manager
	GetAccessService() access.Service
}

// generator can generate a private key compatible with the x509 certificate.
//
// - implements loader.Generator
type certGenerator struct {
	random io.Reader
	curve  elliptic.Curve
}

func newCertGenerator(r io.Reader, c elliptic.Curve) loader.Generator {
	return certGenerator{
		random: r,
		curve:  c,
	}
}

// certGenerator implements loader.Generator. It returns the serialized data of
// a private key generated from the an elliptic curve. The data is formatted as
// a PEM block "EC PRIVATE KEY".
func (g certGenerator) Generate() ([]byte, error) {
	priv, err := ecdsa.GenerateKey(g.curve, g.random)
	if err != nil {
		return nil, xerrors.Errorf("ecdsa: %v", err)
	}

	data, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, xerrors.Errorf("while marshaling: %v", err)
	}

	return data, nil
}

func newKeyGenerator() loader.Generator {
	return keyGenerator{}
}

// keyGenerator is an implementation to generate a private key.
//
// - implements loader.Generator
type keyGenerator struct {
}

// Generate implements loader.Generator. It returns the marshaled data of a
// private key.
func (g keyGenerator) Generate() ([]byte, error) {
	signer := bls.NewSigner()

	data, err := signer.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal signer: %v", err)
	}

	return data, nil
}

// Client is a local client for the manager to read the current identity's nonce
// from the ordering service.
//
// - implements signed.Client
type client struct {
	srvc ordering.Service
	mgr  validation.Service
}

// GetNonce implements signed.Client. It reads the store of the ordering service
// to get the next nonce of the identity and returns it.
func (c client) GetNonce(ident access.Identity) (uint64, error) {
	store := c.srvc.GetStore()

	nonce, err := c.mgr.GetNonce(store, ident)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}

// newAccessStore returns a new access store
func newAccessStore() accessstore {
	return accessstore{
		bucket: make(map[string][]byte),
	}
}

// accessstore is an in-memory store access
//
// - implements store.Readable
// - implements store.Writable
type accessstore struct {
	bucket map[string][]byte
}

// Get implements store.Readable
func (a accessstore) Get(key []byte) ([]byte, error) {
	return a.bucket[string(key)], nil
}

// Set implements store.Writable
func (a accessstore) Set(key, value []byte) error {
	a.bucket[string(key)] = value

	return nil
}

// Delete implements store.Writable
func (a accessstore) Delete(key []byte) error {
	delete(a.bucket, string(key))

	return nil
}

// txClient return monotically increasing nonce
//
// - implements signed.Client
type txClient struct {
	nonce uint64
}

// GetNonce implements signed.Client
func (c *txClient) GetNonce(access.Identity) (uint64, error) {
	res := c.nonce
	c.nonce++
	return res, nil
}
