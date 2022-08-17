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
	n_old := 10
	threshold_old := 10
	minos_old := make([]mino.Mino, n_old)
	dkgs_old := make([]dkg.DKG, n_old)
	addrs_old := make([]mino.Address, n_old)
	pubkeys_old := make([]kyber.Point, len(minos_old))
	minoManager := minoch.NewManager()
	// defining the addresses
	for i := 0; i < n_old; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minos_old[i] = minogrpc
		addrs_old[i] = minogrpc.GetAddress()
	}
	// initializing the pedersen
	for i, mino := range minos_old {
		dkg, pubkey := NewPedersen(mino)
		dkgs_old[i] = dkg
		pubkeys_old[i] = pubkey
	}
	fakeAuthority := NewAuthority(addrs_old, pubkeys_old)
	// initializing the old committee actors
	actors_old := make([]dkg.Actor, n_old)
	for i := 0; i < n_old; i++ {
		actor, err := dkgs_old[i].Listen()
		require.NoError(t, err)
		actors_old[i] = actor
	}
	_, err := actors_old[1].Setup(fakeAuthority, threshold_old)
	require.NoError(t, err, "setting up the firs dkg was not successful")
	// encrypt a message with the old committee public key. the new committee should be able to decrypt
	// it successfully
	message := []byte("Hello world")
	K, C, remainder, err := actors_old[0].Encrypt(message)
	require.NoError(t, err, "encrypting the message was not successful")
	require.Len(t, remainder, 0)
	//setting up the second dkg
	// n_common is the number of nodes that are common between the new and the old committee
	n_common := 10
	// the number of new added nodes. the new committee should have n_common+n_new nodes in totatl
	n_new := 20
	threshold_new := 30
	minos_new := make([]mino.Mino, n_new+n_common)
	dkgs_new := make([]dkg.DKG, n_new+n_common)
	addrs_new := make([]mino.Address, n_new+n_common)
	//the first n_common nodes of  committee are the same as the first n_common nodes of the old committee
	for i := 0; i < n_common; i++ {
		minos_new[i] = minos_old[i]
		addrs_new[i] = minos_old[i].GetAddress()
	}

	pubkeys_new := make([]kyber.Point, len(minos_new))
	// defining the address of the new nodes.
	for i := 0; i < n_new; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr new %d", i))
		minos_new[i+n_common] = minogrpc
		addrs_new[i+n_common] = minogrpc.GetAddress()
	}
	// initializing the pedersen of the new nodes. the common nodes already have a pedersen
	for i := 0; i < n_new; i++ {
		dkg, pubkey := NewPedersen(minos_new[i+n_common])
		dkgs_new[i+n_common] = dkg
		pubkeys_new[i+n_common] = pubkey
	}
	for i := 0; i < n_common; i++ {
		dkgs_new[i] = dkgs_old[i]
		pubkeys_new[i] = pubkeys_old[i]
	}
	// initializing the actor of the new nodes. the common nodes already have an actor
	actors_new := make([]dkg.Actor, n_new+n_common)
	for i := 0; i < n_common; i++ {
		actors_new[i] = actors_old[i]
	}
	for i := 0; i < n_new; i++ {
		actor, err := dkgs_new[i+n_common].Listen()
		require.NoError(t, err)
		actors_new[i+n_common] = actor
	}
	// resharing the committee secret among the new committee
	err = actors_old[0].Reshare(threshold_new, threshold_old, addrs_new, pubkeys_new, pubkeys_old)
	require.NoError(t, err, "Resharing was not successful")
	// comparing the public key of the old and the new committee
	oldPubKey, err := actors_old[0].GetPublicKey()
	require.NoError(t, err)
	for _, actor_new := range actors_new {
		newPubKey, err := actor_new.GetPublicKey()
		// the public key should remain the same
		require.NoError(t, err, "the public key should remain the same")
		newPubKey.Equal(oldPubKey)
		decrypted, err := actor_new.Decrypt(K, C)
		require.NoError(t, err, "decryption was not successful")
		require.Equal(t, message, decrypted, "the new committee should be able to decrypt the messages encrypted by the old committee")
	}
}
