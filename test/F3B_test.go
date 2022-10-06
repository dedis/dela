package integration

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
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

	"go.dedis.ch/dela/mino"

	"go.dedis.ch/dela/mino/minoch"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/xof/keccak"
)

func init() {
	rand.Seed(0)
}

func Test_F3B(t *testing.T) {
	batchSizes := []int{1}
	// numDKGs := []int{3, 10, 20, 30, 50, 70, 100}
	numDKGs := []int{32}

	for _, batchSize := range batchSizes {
		for _, numDKG := range numDKGs {
			t.Run(fmt.Sprintf("batch size %d num dkg %d", batchSize, numDKG), f3bScenario(batchSize, numDKG, 3))
		}
	}
}

func f3bScenario(batchSize, numDKG, numNodes int) func(t *testing.T) {
	return func(t *testing.T) {

		require.Greater(t, numDKG, 0)
		require.Greater(t, numNodes, 0)
		require.GreaterOrEqual(t, numDKG, numNodes)

		to := time.Second * 10 // transaction inclusion timeout

		// set up the dkg
		minos := make([]mino.Mino, numDKG)
		dkgs := make([]dkg.DKG, numDKG)
		addrs := make([]mino.Address, numDKG)

		minoManager := minoch.NewManager()

		// initializing the addresses
		for i := 0; i < numDKG; i++ {
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

		actors := make([]dkg.Actor, numDKG)
		for i := 0; i < numDKG; i++ {
			actor, err := dkgs[i].Listen()
			require.NoError(t, err)
			actors[i] = actor
		}

		fakeAuthority := NewAuthority(addrs, pubkeys)
		start := time.Now()
		_, err := actors[0].Setup(fakeAuthority, numDKG)
		require.NoError(t, err)

		setupTime := time.Since(start)
		t.Logf("setup done in %s", setupTime)

		// setting up the blockchain

		dir, err := os.MkdirTemp("", "dela-integration-test")
		require.NoError(t, err)

		t.Logf("using temps dir %s", dir)

		defer os.RemoveAll(dir)

		nodes := make([]dela, numNodes)

		for i := range nodes {
			nodes[i] = newDelaNode(t, filepath.Join(dir, fmt.Sprintf("node%d", i)), 0)
		}

		nodes[0].Setup(nodes[1:]...)

		l := loader.NewFileLoader(filepath.Join(dir, "private.key"))

		// creating a new client/signer
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
		err = addAndWait(t, to, manager, nodes[0].(cosiDelaNode), args...)
		require.NoError(t, err)

		// creating GBar. we need a generator in order to follow the encryption and
		// decryption protocol of https://arxiv.org/pdf/2205.08529.pdf / we take an
		// agreed data among the participants and embed it as a point. the result is
		// the generator that we are seeking
		var suite = suites.MustFind("Ed25519")
		agreedData := make([]byte, 32)
		_, err = rand.Read(agreedData)
		require.NoError(t, err)
		gBar := suite.Point().Embed(agreedData, keccak.New(agreedData))

		// creating the symmetric keys in batch. we process the transactions in
		// batch to increase the throughput for more information refer to
		// https://arxiv.org/pdf/2205.08529.pdf / page 6 / step 1 (write
		// transaction)

		// the write transaction arguments
		argSlice := make([][]txn.Arg, batchSize)

		var ciphertexts []types.Ciphertext

		// generate random messages to be encrypted
		keys := make([][29]byte, batchSize)
		for i := range keys {
			_, err = rand.Read(keys[i][:])
			require.NoError(t, err)
		}

		start = time.Now()

		// Create a Write instance
		for i := 0; i < batchSize; i++ {
			// Encrypting the symmetric key
			ciphertext, remainder, err := actors[0].VerifiableEncrypt(keys[i][:], gBar)
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

			// creating the transaction and write the data
			argSlice[i] = []txn.Arg{
				{Key: "go.dedis.ch/dela.ContractArg", Value: []byte("go.dedis.ch/dela.Value")},
				{Key: "value:key", Value: []byte("key")},

				{Key: "value:value", Value: Ck},
				{Key: "value:command", Value: []byte("WRITE")},
			}

			// we read the recorded data on the blockchain and make sure that
			// the data was submitted correctly
			err = addAndWait(t, to, manager, nodes[0].(cosiDelaNode), argSlice[i]...)
			require.NoError(t, err)
		}

		submitTime := time.Since(start)
		t.Logf("submit batch: %s", submitTime)

		start = time.Now()

		// decrypting the symmetric key in batch
		decrypted, err := actors[0].VerifiableDecrypt(ciphertexts)
		require.NoError(t, err)

		decryptTime := time.Since(start)
		t.Logf("decrypt batch: %s", decryptTime)

		// make sure that the decryption was correct
		for i := 0; i < batchSize; i++ {
			require.Equal(t, keys[i][:], decrypted[i])
		}

		fmt.Println("Setup,\tSubmit,\tDecrypt")
		fmt.Printf("%d,\t%d,\t%d\n", setupTime.Milliseconds(), submitTime.Milliseconds(), decryptTime.Milliseconds())
	}
}

// -----------------------------------------------------------------------------
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
