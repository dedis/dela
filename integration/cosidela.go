package integration

import (
	"context"
	"crypto/elliptic"
	"crypto/x509"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde/json"
)

// cosiDela defines the interface needed to use a Dela node using cosi.
type cosiDela interface {
	dela

	GetAddr() net.Addr
	GetPublicKey() crypto.PublicKey
	GetPool() pool.Pool
	GetAccessStore() accessstore
	GetTree() hashtree.Tree
}

type cosiDelaNode struct {
	t             *testing.T
	onet          mino.Mino
	ordering      ordering.Service
	cosi          *threshold.Threshold
	txManager     txn.Manager
	addr          net.Addr
	pool          pool.Pool
	accessService access.Service
	accessStore   accessstore
	tree          hashtree.Tree
}

func newDelaNode(t *testing.T, path string, port int) dela {
	err := os.MkdirAll(path, 0700)
	require.NoError(t, err)

	// store
	db, err := kv.New(filepath.Join(path, "dela.db"))
	require.NoError(t, err)

	// mino
	router := tree.NewRouter(minogrpc.NewAddressFactory())
	addr := minogrpc.ParseAddress("127.0.0.1", uint16(port))

	certs := certs.NewDiskStore(db, session.AddressFactory{})

	fload := loader.NewFileLoader(filepath.Join(path, certKeyName))

	keydata, err := fload.LoadOrCreate(newCertGenerator(rand.New(rand.NewSource(0)), elliptic.P521()))
	require.NoError(t, err)

	key, err := x509.ParseECPrivateKey(keydata)
	require.NoError(t, err)

	opts := []minogrpc.Option{
		minogrpc.WithStorage(certs),
		minogrpc.WithCertificateKey(key, key.Public()),
	}

	onet, err := minogrpc.NewMinogrpc(addr, router, opts...)
	require.NoError(t, err)

	// ordering + validation + execution
	fload = loader.NewFileLoader(filepath.Join(path, privateKeyFile))

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

	value.RegisterContract(exec, value.NewContract(valueAccessKey[:], accessService))

	txFac := signed.NewTransactionFactory()
	vs := simple.NewService(exec, txFac)

	pool, err := poolimpl.NewPool(gossip.NewFlat(onet.WithSegment("pool"), txFac))
	require.NoError(t, err)

	tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

	param := cosipbft.ServiceParam{
		Mino:       onet,
		Cosi:       cosi,
		Validation: vs,
		Access:     accessService,
		Pool:       pool,
		DB:         db,
		Tree:       tree,
	}

	err = tree.Load()
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

	srvc, err := cosipbft.NewService(param)
	require.NoError(t, err)

	// tx
	mgr := signed.NewManager(cosi.GetSigner(), client{
		srvc: srvc,
		mgr:  vs,
	})

	// access
	accessStore := newAccessStore()
	contract := accessContract.NewContract(aKey[:], accessService, accessStore)
	accessContract.RegisterContract(exec, contract)

	return cosiDelaNode{
		t:             t,
		onet:          onet,
		ordering:      srvc,
		cosi:          cosi,
		txManager:     mgr,
		addr:          addr,
		pool:          pool,
		accessService: accessService,
		accessStore:   accessStore,
		tree:          tree,
	}
}

// Setup implements dela. It creates the roster, shares the certificate, and
// create an new chain.
func (c cosiDelaNode) Setup(delas ...dela) {
	// share the certificates
	joinable, ok := c.onet.(minogrpc.Joinable)
	require.True(c.t, ok)

	addrStr := c.addr.String()
	token := joinable.GenerateToken(time.Hour)

	certHash, err := joinable.GetCertificateStore().Hash(joinable.GetCertificate())
	require.NoError(c.t, err)

	for _, dela := range delas {
		otherJoinable, ok := dela.GetMino().(minogrpc.Joinable)
		require.True(c.t, ok)

		err = otherJoinable.Join(addrStr, token, certHash)
		require.NoError(c.t, err)
	}

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
	err = extended.Setup(context.Background(), roster)
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

// GetAddr implements cosiDela
func (c cosiDelaNode) GetAddr() net.Addr {
	return c.addr
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
