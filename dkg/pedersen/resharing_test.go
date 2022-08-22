package pedersen

import (
	"fmt"
	"log"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	_ "go.dedis.ch/dela/dkg/pedersen/json"

	//minogrpc "go.dedis.ch/dela/mino/minoch"
	//"go.dedis.ch/dela/mino/router/tree"
	"net/http"
	_ "net/http/pprof"

	"go.dedis.ch/dela/dkg"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	_ "go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/kyber/v3"
)

func init() {
	rand.Seed(0)
}

// this test creats a dkg committee then creats another committee (that can share some nodes with the old committee) and
// then redistributes the secret to the new commitee
func TestResharing(t *testing.T) {

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	// setting up the first dkg
	nOld := 10
	thresholdOld := 10
	minosOld := make([]mino.Mino, nOld)
	dkgsOld := make([]dkg.DKG, nOld)
	addrsOld := make([]mino.Address, nOld)
	pubkeysOld := make([]kyber.Point, len(minosOld))
	minoManager := minoch.NewManager()
	// defining the addresses
	for i := 0; i < nOld; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minosOld[i] = minogrpc
		addrsOld[i] = minogrpc.GetAddress()
	}
	// initializing the pedersen
	for i, mino := range minosOld {
		dkg, pubkey := NewPedersen(mino)
		dkgsOld[i] = dkg
		pubkeysOld[i] = pubkey
	}
	fakeAuthority := NewAuthority(addrsOld, pubkeysOld)
	// initializing the old committee actors
	actorsOld := make([]dkg.Actor, nOld)
	for i := 0; i < nOld; i++ {
		actor, err := dkgsOld[i].Listen()
		require.NoError(t, err)
		actorsOld[i] = actor
	}
	_, err := actorsOld[1].Setup(fakeAuthority, thresholdOld)
	require.NoError(t, err, "setting up the firs dkg was not successful")
	// encrypt a message with the old committee public key. the new committee should be able to decrypt
	// it successfully
	message := []byte("Hello world")
	K, C, remainder, err := actorsOld[0].Encrypt(message)
	require.NoError(t, err, "encrypting the message was not successful")
	require.Len(t, remainder, 0)
	//setting up the second dkg
	// nCommon is the number of nodes that are common between the new and the old committee
	nCommon := 5
	// the number of new added nodes. the new committee should have nCommon+nNew nodes in totatl
	nNew := 50
	thresholdNew := 55
	minosNew := make([]mino.Mino, nNew+nCommon)
	dkgsNew := make([]dkg.DKG, nNew+nCommon)
	addrsNew := make([]mino.Address, nNew+nCommon)
	//the first nCommon nodes of  committee are the same as the first nCommon nodes of the old committee
	for i := 0; i < nCommon; i++ {
		minosNew[i] = minosOld[i]
		addrsNew[i] = minosOld[i].GetAddress()
	}

	pubkeysNew := make([]kyber.Point, len(minosNew))
	// defining the address of the new nodes.
	for i := 0; i < nNew; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr new %d", i))
		minosNew[i+nCommon] = minogrpc
		addrsNew[i+nCommon] = minogrpc.GetAddress()
	}
	// initializing the pedersen of the new nodes. the common nodes already have a pedersen
	for i := 0; i < nNew; i++ {
		dkg, pubkey := NewPedersen(minosNew[i+nCommon])
		dkgsNew[i+nCommon] = dkg
		pubkeysNew[i+nCommon] = pubkey
	}
	for i := 0; i < nCommon; i++ {
		dkgsNew[i] = dkgsOld[i]
		pubkeysNew[i] = pubkeysOld[i]
	}
	// initializing the actor of the new nodes. the common nodes already have an actor
	actorsNew := make([]dkg.Actor, nNew+nCommon)
	for i := 0; i < nCommon; i++ {
		actorsNew[i] = actorsOld[i]
	}
	for i := 0; i < nNew; i++ {
		actor, err := dkgsNew[i+nCommon].Listen()
		require.NoError(t, err)
		actorsNew[i+nCommon] = actor
	}
	// resharing the committee secret among the new committee
	fakeAuthority = NewAuthority(addrsNew, pubkeysNew)
	err = actorsOld[0].Reshare(fakeAuthority, thresholdNew)
	require.NoError(t, err, "Resharing was not successful")
	// comparing the public key of the old and the new committee
	oldPubKey, err := actorsOld[0].GetPublicKey()
	require.NoError(t, err)
	for _, actorNew := range actorsNew {
		newPubKey, err := actorNew.GetPublicKey()
		// the public key should remain the same
		require.NoError(t, err, "the public key should remain the same")
		newPubKey.Equal(oldPubKey)
		decrypted, err := actorNew.Decrypt(K, C)
		require.NoError(t, err, "decryption was not successful")
		require.Equal(t, message, decrypted, "the new committee should be able to decrypt the messages encrypted by the old committee")
	}
}
