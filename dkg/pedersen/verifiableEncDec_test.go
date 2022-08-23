package pedersen

import (
	"fmt"
	"log"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	_ "net/http/pprof"

	"net/http"

	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"

	"go.dedis.ch/dela/mino/minoch"
	_ "go.dedis.ch/dela/mino/minoch"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/xof/keccak"
)

func init() {
	rand.Seed(0)
}

func Test_verifiableEncDec(t *testing.T) {

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	// setting up the dkg
	n := 10
	threshold := 10
	batchSize := 100
	workerNum := 8

	minos := make([]mino.Mino, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	// creating GBar. we need a generator in order to follow the encryption and decryption protocol of https://arxiv.org/pdf/2205.08529.pdf /
	// we take an agreed data among the participants and embed it as a point. the result is the generator that we are seeking
	agreedData := make([]byte, 32)
	_, err := rand.Read(agreedData)
	require.NoError(t, err)
	GBar := suite.Point().Embed(agreedData, keccak.New(agreedData))

	minoManager := minoch.NewManager()

	for i := 0; i < n; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minos[i] = minogrpc
		addrs[i] = minogrpc.GetAddress()
	}

	pubkeys := make([]kyber.Point, len(minos))

	for i, mino := range minos {
		dkg, pubkey := NewPedersen(mino)
		dkgs[i] = dkg
		pubkeys[i] = pubkey
	}

	fakeAuthority := NewAuthority(addrs, pubkeys)

	actors := make([]dkg.Actor, n)
	for i := 0; i < n; i++ {
		actor, err := dkgs[i].Listen()
		require.NoError(t, err)
		actors[i] = actor
	}

	_, err = actors[0].Setup(fakeAuthority, threshold)
	require.NoError(t, err)

	//generating random messages in batch and encrypt them
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	keys := make([][]byte, batchSize)
	var ciphertexts []types.Ciphertext
	for i := 0; i < batchSize; i++ {
		keys[i] = make([]byte, 29)
		for j := range keys[i] {
			keys[i][j] = letterBytes[rand.Intn(len(letterBytes))]
		}
		ciphertext, remainder, err := actors[0].VerifiableEncrypt(keys[i], GBar)
		require.NoError(t, err)
		require.Len(t, remainder, 0)
		ciphertexts = append(ciphertexts, ciphertext)
	}
	// decryopting the batch ciphertext message
	decrypted, err := actors[0].VerifiableDecrypt(ciphertexts, workerNum)
	require.NoError(t, err)
	for i := 0; i < batchSize; i++ {
		require.Equal(t, keys[i], decrypted[i])
	}

}
