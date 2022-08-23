package pedersen

import (
	"fmt"
	"log"
	"math/rand"
	"testing"

	"net/http"
	_ "net/http/pprof"

	"github.com/stretchr/testify/require"
	_ "go.dedis.ch/dela/dkg/pedersen/json"

	"go.dedis.ch/dela/mino/minogrpc"
	_ "go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/router/tree"

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
// then redistributes the secret to the new commitee.
// using minoch as the underlying network
func TestResharing_minoch(t *testing.T) {

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	// setting up the first dkg
	nOld := 3
	thresholdOld := 3

	minosOld := make([]mino.Mino, nOld)
	dkgsOld := make([]dkg.DKG, nOld)
	addrsOld := make([]mino.Address, nOld)
	pubkeysOld := make([]kyber.Point, len(minosOld))
	minoManager := minoch.NewManager()

	// defining the addresses
	for i := 0; i < nOld; i++ {
		mino := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minosOld[i] = mino
		addrsOld[i] = mino.GetAddress()
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
	nCommon := 1
	// the number of new added nodes. the new committee should have nCommon+nNew nodes in totatl
	nNew := 2
	thresholdNew := 3
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

// this test creats a dkg committee then creats another committee (that can share some nodes with the old committee) and
// then redistributes the secret to the new commitee
// using minogrpc as the underlying network
func TestResharing_minogrpc(t *testing.T) {

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

	// defining the addresses
	for i := 0; i < nOld; i++ {
		addr := minogrpc.ParseAddress("127.0.0.1", 0)
		minogrpc, err := minogrpc.NewMinogrpc(addr, nil, tree.NewRouter(minogrpc.NewAddressFactory()))
		require.NoError(t, err)
		defer minogrpc.GracefulStop()

		minosOld[i] = minogrpc
		addrsOld[i] = minogrpc.GetAddress()
	}

	// initializing the pedersen
	for i, mino := range minosOld {
		for _, m := range minosOld {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(), m.(*minogrpc.Minogrpc).GetCertificateChain())
		}

		dkg, pubkey := NewPedersen(mino.(*minogrpc.Minogrpc))

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
	nNew := 20
	thresholdNew := 25

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
		addr := minogrpc.ParseAddress("127.0.0.1", 0)
		minogrpc, err := minogrpc.NewMinogrpc(addr, nil, tree.NewRouter(minogrpc.NewAddressFactory()))
		require.NoError(t, err)
		defer minogrpc.GracefulStop()
		minosNew[i+nCommon] = minogrpc
		addrsNew[i+nCommon] = minogrpc.GetAddress()
	}

	//the first nCommon nodes of  committee are the same as the first nCommon nodes of the old committee
	for i := 0; i < nCommon; i++ {
		dkgsNew[i] = dkgsOld[i]
		pubkeysNew[i] = pubkeysOld[i]
	}

	// initializing the pedersen of the new nodes. the common nodes already have a pedersen
	for i, mino := range minosNew[nCommon:] {
		for _, m := range minosNew {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(), m.(*minogrpc.Minogrpc).GetCertificateChain())
			m.(*minogrpc.Minogrpc).GetCertificateStore().Store(mino.GetAddress(), mino.(*minogrpc.Minogrpc).GetCertificateChain())
		}
		for _, m := range minosOld[nCommon:] {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(), m.(*minogrpc.Minogrpc).GetCertificateChain())
			m.(*minogrpc.Minogrpc).GetCertificateStore().Store(mino.GetAddress(), mino.(*minogrpc.Minogrpc).GetCertificateChain())
		}
		dkg, pubkey := NewPedersen(mino.(*minogrpc.Minogrpc))
		dkgsNew[i+nCommon] = dkg
		pubkeysNew[i+nCommon] = pubkey
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
