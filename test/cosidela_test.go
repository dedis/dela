package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/contracts/value"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	poolimpl "go.dedis.ch/dela/core/txn/pool/gossip"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minows"
	"go.dedis.ch/dela/mino/minows/key"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

const minoWS = "ws"

const privateKeyFile = "private.key"

// cosiDela defines the interface needed to use a Dela node using cosi.
type cosiDela interface {
	dela

	GetPublicKey() crypto.PublicKey
	GetPool() pool.Pool
	GetAccessStore() accessstore
	GetTree() hashtree.Tree
}

// cosiDelaNode represents a Dela node using cosi pbft
//
// - implements dela
type cosiDelaNode struct {
	t             require.TestingT
	onet          mino.Mino
	ordering      ordering.Service
	cosi          *threshold.Threshold
	txManager     txn.Manager
	pool          pool.Pool
	accessService access.Service
	accessStore   accessstore
	tree          hashtree.Tree
}

func newDelaNode(t require.TestingT, path string, port int, kind string) dela {
	err := os.MkdirAll(path, 0700)
	require.NoError(t, err)

	// store
	db, err := kv.New(filepath.Join(path, "dela.db"))
	require.NoError(t, err)

	// mino
	var onet mino.Mino
	switch kind {
	case minoWS:
		listen, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ws", port))
		require.NoError(t, err)

		privKey, _ := minokey.NewKey(db)

		manager := minows.NewManager()
		onet, err = minows.NewMinows(manager, listen, nil, privKey)
		require.NoError(t, err)
	}
	onet.GetAddress()

	// ordering + validation + execution
	fload := loader.NewFileLoader(filepath.Join(path, privateKeyFile))

	signerdata, err := fload.LoadOrCreate(newKeyGenerator())
	require.NoError(t, err)

	signer, err := bls.NewSignerFromBytes(signerdata)
	require.NoError(t, err)

	cosi := threshold.NewThreshold(onet.WithSegment("cosi"), signer)
	cosi.SetThreshold(threshold.ByzantineThreshold)

	exec := native.NewExecution()
	accessService := darc.NewService(json.NewContext())

	rosterFac := authority.NewFactory(onet.GetAddressFactory(), cosi.GetPublicKeyFactory())
	cosipbft.RegisterRosterContract(exec, rosterFac, accessService)

	value.RegisterContract(exec, value.NewContract(accessService))

	txFac := signed.NewTransactionFactory()
	vs := simple.NewService(exec, txFac)

	newPool, err := poolimpl.NewPool(gossip.NewFlat(onet.WithSegment("pool"), txFac))
	require.NoError(t, err)

	newTree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

	param := cosipbft.ServiceParam{
		Mino:       onet,
		Cosi:       cosi,
		Validation: vs,
		Access:     accessService,
		Pool:       newPool,
		DB:         db,
		Tree:       newTree,
	}

	err = newTree.Load()
	require.NoError(t, err)

	genstore := blockstore.NewGenesisDiskStore(db, types.NewGenesisFactory(rosterFac))

	err = genstore.Load()
	require.NoError(t, err)

	blockFac := types.NewBlockFactory(vs.GetFactory())
	csFac := authority.NewChangeSetFactory(onet.GetAddressFactory(), cosi.GetPublicKeyFactory())
	linkFac := types.NewLinkFactory(blockFac, cosi.GetSignatureFactory(), csFac)

	blocks := blockstore.NewDiskStore(db, linkFac)

	err = blocks.Load()
	require.NoError(t, err)

	srvc, err := cosipbft.NewServiceStruct(param)
	require.NoError(t, err)
	require.NotNil(t, srvc)
	srvc.SetTimeouts(1*time.Second, 3*time.Second, 10*time.Second)
	cosipbft.NewServiceStart(srvc)

	// tx
	mgr := signed.NewManager(cosi.GetSigner(), client{
		srvc: srvc,
		mgr:  vs,
	})

	// access
	accessStore := newAccessStore()
	contract := accessContract.NewContract(accessService, accessStore)
	accessContract.RegisterContract(exec, contract)

	return cosiDelaNode{
		t:             t,
		onet:          onet,
		ordering:      srvc,
		cosi:          cosi,
		txManager:     mgr,
		pool:          newPool,
		accessService: accessService,
		accessStore:   accessStore,
		tree:          newTree,
	}
}

// Setup implements dela. It creates the roster, shares the certificate, and
// create a new chain.
func (c cosiDelaNode) Setup(kind string, delas ...dela) {
	type extendedService interface {
		GetRoster() (authority.Authority, error)
		Setup(ctx context.Context, ca crypto.CollectiveAuthority) error
	}

	// make roster
	extended, ok := c.GetOrdering().(extendedService)
	require.True(c.t, ok)

	minoAddrs := make([]mino.Address, len(delas)+1)
	pubKeys := make([]crypto.PublicKey, len(delas)+1)

	for i, dela := range delas {
		minoAddr := dela.GetMino().GetAddress()

		d, ok := dela.(cosiDela)
		require.True(c.t, ok)

		pubkey := d.GetPublicKey()

		minoAddrs[i+1] = minoAddr
		pubKeys[i+1] = pubkey
	}

	minoAddrs[0] = c.onet.GetAddress()
	pubKeys[0] = c.cosi.GetSigner().GetPublicKey()

	roster := authority.New(minoAddrs, pubKeys)

	// create chain
	err := extended.Setup(context.Background(), roster)
	require.NoError(c.t, err)
}

// GetMino implements dela
func (c cosiDelaNode) GetMino() mino.Mino {
	return c.onet
}

// GetOrdering implements dela
func (c cosiDelaNode) GetOrdering() ordering.Service {
	return c.ordering
}

// GetTxManager implements dela
func (c cosiDelaNode) GetTxManager() txn.Manager {
	return c.txManager
}

// GetAccessService implements dela
func (c cosiDelaNode) GetAccessService() access.Service {
	return c.accessService
}

// GetPublicKey  implements cosiDela
func (c cosiDelaNode) GetPublicKey() crypto.PublicKey {
	return c.cosi.GetSigner().GetPublicKey()
}

// GetPool implements cosiDela
func (c cosiDelaNode) GetPool() pool.Pool {
	return c.pool
}

// GetAccessStore implements cosiDela
func (c cosiDelaNode) GetAccessStore() accessstore {
	return c.accessStore
}

// GetTree implements cosiDela
func (c cosiDelaNode) GetTree() hashtree.Tree {
	return c.tree
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

// txClient returns always a 0 nonce
//
// - implements signed.Client
type txClient struct {
}

// GetNonce implements signed.Client
func (c *txClient) GetNonce(access.Identity) (uint64, error) {
	return 0, nil
}
