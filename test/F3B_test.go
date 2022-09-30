package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "net/http/pprof"

	"github.com/stretchr/testify/require"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/dkg/pedersen"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"

	"net/http"

	"go.dedis.ch/dela/mino"

	"go.dedis.ch/dela/mino/minoch"
	_ "go.dedis.ch/dela/mino/minoch"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/xof/keccak"
)

func init() {
	rand.Seed(0)
}

func Test_F3B(t *testing.T) {

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	numWorkersSlice := []int{2, 4, 8}
	batchSizeSlice := []int{8, 32, 64}

	// logging the result
	// f, err := os.OpenFile("../logs/test.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	// require.NoError(t, err)
	// defer f.Close()
	// wrt := io.MultiWriter(os.Stdout, f)
	// log.SetOutput(wrt)

	//start := time.Now()

	fmt.Println("setting up the dkg ...")
	n := 32
	threshold := 32

	// set up the dkg
	minos := make([]mino.Mino, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	minoManager := minoch.NewManager()

	// initializing the addresses
	for i := 0; i < n; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minos[i] = minogrpc
		addrs[i] = minogrpc.GetAddress()
	}

	pubkeys := make([]kyber.Point, len(minos))

	for i, mino := range minos {
		dkg, pubkey := pedersen.NewPedersen(mino)
		dkgs[i] = dkg
		pubkeys[i] = pubkey
	}

	actors := make([]dkg.Actor, n)
	for i := 0; i < n; i++ {
		actor, err := dkgs[i].Listen()
		require.NoError(t, err)
		actors[i] = actor
	}

	fakeAuthority := NewAuthority(addrs, pubkeys)
	_, err := actors[0].Setup(fakeAuthority, threshold)
	require.NoError(t, err)

	// timing the dkg set up
	//dkgSettingTime := time.Since(start)

	//setting up the blockchain
	fmt.Println("setting up the dela blockchain ...")
	dir, err := ioutil.TempDir(os.TempDir(), "dela-integration-test")
	require.NoError(t, err)

	t.Logf("using temps dir %s", dir)

	defer os.RemoveAll(dir)

	// running the dela blockchain with 3 nodes
	nodes := []dela{
		newDelaNode(t, filepath.Join(dir, "node1"), 0),
		newDelaNode(t, filepath.Join(dir, "node2"), 0),
		newDelaNode(t, filepath.Join(dir, "node3"), 0),
	}

	nodes[0].Setup(nodes[1:]...)

	l := loader.NewFileLoader(filepath.Join(dir, "private.key"))

	//creating a new client/signer
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

	// sending the grant transaction to the blockchain
	args := []txn.Arg{
		{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Access")},
		{Key: "access:grant_id", Value: []byte(hex.EncodeToString(valueAccessKey[:]))},
		{Key: "access:grant_contract", Value: []byte("go.dedis.ch/dela.Value")},
		{Key: "access:grant_command", Value: []byte("all")},
		{Key: "access:identity", Value: []byte(base64.StdEncoding.EncodeToString(pubKeyBuf))},
		{Key: "access:command", Value: []byte("GRANT")},
	}

	// waiting for the confirmation of the transaction
	addAndWait(t, manager, nodes[0].(cosiDelaNode), args...)

	// creating GBar. we need a generator in order to follow the encryption and decryption protocol of https://arxiv.org/pdf/2205.08529.pdf /
	// we take an agreed data among the participants and embed it as a point. the result is the generator that we are seeking
	var suite = suites.MustFind("Ed25519")
	agreedData := make([]byte, 32)
	_, err = rand.Read(agreedData)
	require.NoError(t, err)
	gBar := suite.Point().Embed(agreedData, keccak.New(agreedData))

	// creating the symmetric keys in batch. we process the transactions in batch to increase the throughput
	// for more information refer to https://arxiv.org/pdf/2205.08529.pdf / page 6 / step 1 (write transaction)
	fmt.Println("encrypting the data ...")

	for i, batchSize := range batchSizeSlice {
		// the write transaction arguments
		argSlice := make([][]txn.Arg, batchSize)

		numWorkers := numWorkersSlice[i]
		var ciphertexts []types.Ciphertext

		const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
		keys := make([][]byte, batchSize)
		//Create a Write instance
		for i := 0; i < batchSize; i++ {
			keys[i] = make([]byte, 29)
			for j := range keys[i] {
				keys[i][j] = letterBytes[rand.Intn(len(letterBytes))]
			}

			// Encrypting the symmetric key
			ciphertext, remainder, err := actors[0].VerifiableEncrypt(keys[i], gBar)
			require.NoError(t, err)
			require.Len(t, remainder, 0)

			ciphertexts = append(ciphertexts, ciphertext)

			// converting the kyber.Point or kyber.Scalar to bytes
			Cbytes, err := ciphertext.C.MarshalBinary()
			require.NoError(t, err)
			Ubytes, err := ciphertext.K.MarshalBinary()
			require.NoError(t, err)
			Ubarbytes, err := ciphertext.UBar.MarshalBinary()
			require.NoError(t, err)
			Ebytes, err := ciphertext.E.MarshalBinary()
			require.NoError(t, err)
			Fbytes, err := ciphertext.F.MarshalBinary()
			require.NoError(t, err)

			// put all the data together
			Ck := append(Cbytes[:], Ubytes[:]...)
			Ck = append(Ck, Ubarbytes[:]...)
			Ck = append(Ck, Ebytes[:]...)
			Ck = append(Ck, Fbytes[:]...)

			//creating the transaction and write the data
			argSlice[i] = []txn.Arg{
				{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Value")},
				{Key: "value:key", Value: []byte("key")},

				{Key: "value:value", Value: Ck},
				{Key: "value:command", Value: []byte("WRITE")},
			}

			// we read the recorded data on the blockchain and make sure that the data was submitted corrrectly
			readCk := addAndWait(t, manager, nodes[0].(cosiDelaNode), argSlice[i]...)
			require.Equal(t, readCk, Ck, "correct Ck should be retrived from the blockchain")

		}

		fmt.Println("decrypting the data ...")
		// decryopting the symmetric key in batch
		decrypted, err := actors[0].VerifiableDecrypt(ciphertexts, numWorkers)
		require.NoError(t, err)

		// make sure that the decryption was correct
		fmt.Println("verify the decryption ...")
		for i := 0; i < batchSize; i++ {
			require.Equal(t, keys[i], decrypted[i])
		}

		// log.Printf("number of nodes in the secret committee = %d , number of workers = %d , batch size = %d ,Key reconstruction time = %v s, throughput =  %v tx/s , dkg setup time = %v s",
		// 	n, numWorkers,
		// 	batchSize, elapsed/1000, 1000*float32(batchSize)/elapsed, float32(dkgSettingTime/time.Second))
	}

}

// Utility functions

func addAndWait(t *testing.T, manager txn.Manager, node cosiDelaNode, args ...txn.Arg) []byte {

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

			if bytes.Equal(tx.GetID(), tx.GetID()) {
				accepted, err := event.Transactions[0].GetStatus()
				require.Empty(t, err)
				require.True(t, accepted)
				// get the args of the transaction
				return event.Transactions[0].GetTransaction().GetArg("value:value")
			}
		}
	}

	t.Error("transaction not found")
	return nil
}

// Utility functions

//
// Collective authority
//

// CollectiveAuthority is a fake implementation of the cosi.CollectiveAuthority
// interface.
type CollectiveAuthority struct {
	crypto.CollectiveAuthority
	addrs   []mino.Address
	pubkeys []kyber.Point
	signers []crypto.Signer
}

// NewAuthority returns a new collective authority of n members with new signers
// generated by g.
func NewAuthority(addrs []mino.Address, pubkeys []kyber.Point) CollectiveAuthority {
	signers := make([]crypto.Signer, len(pubkeys))
	for i, pubkey := range pubkeys {
		signers[i] = newFakeSigner(pubkey)
	}

	return CollectiveAuthority{
		pubkeys: pubkeys,
		addrs:   addrs,
		signers: signers,
	}
}

// GetPublicKey implements cosi.CollectiveAuthority.
func (ca CollectiveAuthority) GetPublicKey(addr mino.Address) (crypto.PublicKey, int) {

	for i, address := range ca.addrs {
		if address.Equal(addr) {
			return ed25519.NewPublicKeyFromPoint(ca.pubkeys[i]), i
		}
	}
	return nil, -1
}

// Len implements mino.Players.
func (ca CollectiveAuthority) Len() int {
	return len(ca.pubkeys)
}

// AddressIterator implements mino.Players.
func (ca CollectiveAuthority) AddressIterator() mino.AddressIterator {
	return fake.NewAddressIterator(ca.addrs)
}

func (ca CollectiveAuthority) PublicKeyIterator() crypto.PublicKeyIterator {
	return fake.NewPublicKeyIterator(ca.signers)
}

func newFakeSigner(pubkey kyber.Point) fakeSigner {
	return fakeSigner{
		pubkey: pubkey,
	}
}

// fakeSigner is a fake signer
//
// - implements crypto.Signer
type fakeSigner struct {
	crypto.Signer
	pubkey kyber.Point
}

// GetPublicKey implements crypto.Signer
func (s fakeSigner) GetPublicKey() crypto.PublicKey {
	return ed25519.NewPublicKeyFromPoint(s.pubkey)
}
