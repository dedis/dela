package pedersen

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/router/tree"

	"go.dedis.ch/dela/dkg"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"

	"go.dedis.ch/kyber/v3"
)

func init() {
	rand.Seed(0)
}

// This test creates a dkg committee then creates another committee (that can
// share some nodes with the old committee) and then redistributes the secret to
// the new commitee. Using minoch as the underlying network
func TestResharing_minoch(t *testing.T) {

	// Setting up the first dkg
	nOld := 15
	thresholdOld := nOld

	minosOld := make([]mino.Mino, nOld)
	dkgsOld := make([]dkg.DKG, nOld)
	addrsOld := make([]mino.Address, nOld)
	pubkeysOld := make([]kyber.Point, len(minosOld))
	minoManager := minoch.NewManager()

	// Defining the addresses
	for i := 0; i < nOld; i++ {
		mino := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minosOld[i] = mino
		addrsOld[i] = mino.GetAddress()
	}

	// Initializing the pedersen
	for i, mino := range minosOld {
		dkg, pubkey := NewPedersen(mino)
		dkgsOld[i] = dkg
		pubkeysOld[i] = pubkey
	}

	fakeAuthority := NewAuthority(addrsOld, pubkeysOld)

	// Initializing the old committee actors
	actorsOld := make([]dkg.Actor, nOld)

	for i := 0; i < nOld; i++ {
		actor, err := dkgsOld[i].Listen()
		require.NoError(t, err)
		actorsOld[i] = actor
	}

	_, err := actorsOld[1].Setup(fakeAuthority, thresholdOld)
	require.NoError(t, err, "setting up the firs dkg was not successful")

	t.Log("setup done")

	// Encrypt a message with the old committee public key. The new committee
	// should be able to decrypt it successfully
	message := []byte("Hello world")
	K, C, remainder, err := actorsOld[0].Encrypt(message)
	require.NoError(t, err, "encrypting the message was not successful")
	require.Len(t, remainder, 0)

	// Setting up the second dkg nCommon is the number of nodes that are common
	// between the new and the old committee.
	nCommon := 5

	// The number of new added nodes. the new committee should have nCommon+nNew
	// nodes in total.
	nNew := 10
	thresholdNew := nCommon + nNew
	minosNew := make([]mino.Mino, nNew+nCommon)
	dkgsNew := make([]dkg.DKG, nNew+nCommon)
	addrsNew := make([]mino.Address, nNew+nCommon)

	// The first nCommon nodes of  committee are the same as the first nCommon
	// nodes of the old committee
	for i := 0; i < nCommon; i++ {
		minosNew[i] = minosOld[i]
		addrsNew[i] = minosOld[i].GetAddress()
	}

	pubkeysNew := make([]kyber.Point, len(minosNew))

	// Defining the address of the new nodes.
	for i := 0; i < nNew; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr new %d", i))
		minosNew[i+nCommon] = minogrpc
		addrsNew[i+nCommon] = minogrpc.GetAddress()
	}

	// Initializing the pedersen of the new nodes. the common nodes already have
	// a pedersen
	for i := 0; i < nNew; i++ {
		dkg, pubkey := NewPedersen(minosNew[i+nCommon])
		dkgsNew[i+nCommon] = dkg
		pubkeysNew[i+nCommon] = pubkey
	}

	for i := 0; i < nCommon; i++ {
		dkgsNew[i] = dkgsOld[i]
		pubkeysNew[i] = pubkeysOld[i]
	}

	// Initializing the actor of the new nodes. the common nodes already have an
	// actor
	actorsNew := make([]dkg.Actor, nNew+nCommon)

	for i := 0; i < nCommon; i++ {
		actorsNew[i] = actorsOld[i]
	}

	for i := 0; i < nNew; i++ {
		actor, err := dkgsNew[i+nCommon].Listen()
		require.NoError(t, err)
		actorsNew[i+nCommon] = actor
	}

	// Resharing the committee secret among the new committee
	fakeAuthority = NewAuthority(addrsNew, pubkeysNew)
	err = actorsOld[0].Reshare(fakeAuthority, thresholdNew)
	require.NoError(t, err, "Resharing was not successful")

	// Comparing the public key of the old and the new committee
	oldPubKey, err := actorsOld[0].GetPublicKey()
	require.NoError(t, err)

	for _, actorNew := range actorsNew {
		newPubKey, err := actorNew.GetPublicKey()

		// The public key should remain the same
		require.NoError(t, err, "the public key should remain the same")
		newPubKey.Equal(oldPubKey)
		decrypted, err := actorNew.Decrypt(K, C)
		require.NoError(t, err, "decryption was not successful")
		require.Equal(t, message, decrypted, "the new committee should be able "+
			"to decrypt the messages encrypted by the old committee")
	}

}

// This test creats a dkg committee then creats another committee (that can
// share some nodes with the old committee) and then redistributes the secret to
// the new commitee. Using minogrpc as the underlying network
func TestResharing_minogrpc(t *testing.T) {

	// Setting up the first dkg
	nOld := 10
	thresholdOld := 10

	minosOld := make([]mino.Mino, nOld)
	dkgsOld := make([]dkg.DKG, nOld)
	addrsOld := make([]mino.Address, nOld)
	pubkeysOld := make([]kyber.Point, len(minosOld))

	// Defining the addresses
	for i := 0; i < nOld; i++ {
		addr := minogrpc.ParseAddress("127.0.0.1", 0)
		minogrpc, err := minogrpc.NewMinogrpc(addr, nil, tree.NewRouter(minogrpc.NewAddressFactory()))
		require.NoError(t, err)
		defer minogrpc.GracefulStop()

		minosOld[i] = minogrpc
		addrsOld[i] = minogrpc.GetAddress()
	}

	// Initializing the pedersen
	for i, mino := range minosOld {
		for _, m := range minosOld {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(),
				m.(*minogrpc.Minogrpc).GetCertificateChain())
		}

		dkg, pubkey := NewPedersen(mino.(*minogrpc.Minogrpc))

		dkgsOld[i] = dkg
		pubkeysOld[i] = pubkey
	}

	fakeAuthority := NewAuthority(addrsOld, pubkeysOld)

	// Initializing the old committee actors
	actorsOld := make([]dkg.Actor, nOld)

	for i := 0; i < nOld; i++ {
		actor, err := dkgsOld[i].Listen()
		require.NoError(t, err)
		actorsOld[i] = actor
	}

	_, err := actorsOld[1].Setup(fakeAuthority, thresholdOld)
	require.NoError(t, err, "setting up the firs dkg was not successful")

	// Encrypt a message with the old committee public key. the new committee
	// should be able to decrypt it successfully
	message := []byte("Hello world")
	K, C, remainder, err := actorsOld[0].Encrypt(message)
	require.NoError(t, err, "encrypting the message was not successful")
	require.Len(t, remainder, 0)

	// Setting up the second dkg. nCommon is the number of nodes that are common
	// between the new and the old committee
	nCommon := 5

	// The number of new added nodes. the new committee should have nCommon+nNew
	// nodes in totatl
	nNew := 20
	thresholdNew := nCommon + nNew

	minosNew := make([]mino.Mino, nNew+nCommon)
	dkgsNew := make([]dkg.DKG, nNew+nCommon)
	addrsNew := make([]mino.Address, nNew+nCommon)

	// The first nCommon nodes of  committee are the same as the first nCommon
	// nodes of the old committee
	for i := 0; i < nCommon; i++ {
		minosNew[i] = minosOld[i]
		addrsNew[i] = minosOld[i].GetAddress()
	}

	pubkeysNew := make([]kyber.Point, len(minosNew))

	// Defining the address of the new nodes.
	for i := 0; i < nNew; i++ {
		addr := minogrpc.ParseAddress("127.0.0.1", 0)
		minogrpc, err := minogrpc.NewMinogrpc(addr, nil, tree.NewRouter(minogrpc.NewAddressFactory()))
		require.NoError(t, err)
		defer minogrpc.GracefulStop()

		minosNew[i+nCommon] = minogrpc
		addrsNew[i+nCommon] = minogrpc.GetAddress()
	}

	// The first nCommon nodes of  committee are the same as the first nCommon
	// nodes of the old committee
	for i := 0; i < nCommon; i++ {
		dkgsNew[i] = dkgsOld[i]
		pubkeysNew[i] = pubkeysOld[i]
	}

	// Initializing the pedersen of the new nodes. the common nodes already have
	// a pedersen
	for i, mino := range minosNew[nCommon:] {
		for _, m := range minosNew {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(),
				m.(*minogrpc.Minogrpc).GetCertificateChain())
			m.(*minogrpc.Minogrpc).GetCertificateStore().Store(mino.GetAddress(),
				mino.(*minogrpc.Minogrpc).GetCertificateChain())
		}
		for _, m := range minosOld[nCommon:] {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(),
				m.(*minogrpc.Minogrpc).GetCertificateChain())
			m.(*minogrpc.Minogrpc).GetCertificateStore().Store(mino.GetAddress(),
				mino.(*minogrpc.Minogrpc).GetCertificateChain())
		}
		dkg, pubkey := NewPedersen(mino.(*minogrpc.Minogrpc))
		dkgsNew[i+nCommon] = dkg
		pubkeysNew[i+nCommon] = pubkey
	}

	// Initializing the actor of the new nodes. the common nodes already have an
	// actor
	actorsNew := make([]dkg.Actor, nNew+nCommon)

	for i := 0; i < nCommon; i++ {
		actorsNew[i] = actorsOld[i]
	}
	for i := 0; i < nNew; i++ {
		actor, err := dkgsNew[i+nCommon].Listen()
		require.NoError(t, err)
		actorsNew[i+nCommon] = actor
	}

	// Resharing the committee secret among the new committee
	fakeAuthority = NewAuthority(addrsNew, pubkeysNew)
	err = actorsOld[0].Reshare(fakeAuthority, thresholdNew)
	require.NoError(t, err, "Resharing was not successful")

	// Comparing the public key of the old and the new committee
	oldPubKey, err := actorsOld[0].GetPublicKey()
	require.NoError(t, err)

	for _, actorNew := range actorsNew {
		newPubKey, err := actorNew.GetPublicKey()

		// The public key should remain the same
		require.NoError(t, err, "the public key should remain the same")
		newPubKey.Equal(oldPubKey)
		decrypted, err := actorNew.Decrypt(K, C)
		require.NoError(t, err, "decryption was not successful")
		require.Equal(t, message, decrypted, "the new committee should be able "+
			"to decrypt the messages encrypted by the old committee")
	}
}

// This test creates a dkg committee then creates another committee (that can
// share some nodes with the old committee) and then redistributes the secret to
// the new commitee; later a third committee is built which has a common node
// with both the first and second committee and shares another node only with
// second committee. The third committee also has a brand new node.
// The secret is redistributed among the third committee
func TestResharingTwice(t *testing.T) {

	// Setting up the first dkg
	n1 := 5
	threshold1 := 5

	minos1 := make([]mino.Mino, n1)
	dkgs1 := make([]dkg.DKG, n1)
	addrs1 := make([]mino.Address, n1)
	pubkeys1 := make([]kyber.Point, len(minos1))
	minoManager := minoch.NewManager()

	// Defining the addresses
	for i := 0; i < n1; i++ {
		mino := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minos1[i] = mino
		addrs1[i] = mino.GetAddress()
	}

	// Initializing the pedersen
	for i, mino := range minos1 {
		dkg, pubkey := NewPedersen(mino)
		dkgs1[i] = dkg
		pubkeys1[i] = pubkey
	}

	fakeAuthority := NewAuthority(addrs1, pubkeys1)

	// Initializing the old committee actors
	actors1 := make([]dkg.Actor, n1)

	for i := 0; i < n1; i++ {
		actor, err := dkgs1[i].Listen()
		require.NoError(t, err)
		actors1[i] = actor
	}

	_, err := actors1[1].Setup(fakeAuthority, threshold1)
	require.NoError(t, err, "setting up the firs dkg was not successful")

	// Encrypt a message with the old committee public key. the new committee
	// should be able to decrypt it successfully
	message := []byte("Hello world")
	K, C, remainder, err := actors1[0].Encrypt(message)
	require.NoError(t, err, "encrypting the message was not successful")
	require.Len(t, remainder, 0)

	// Setting up the second dkg nCommon is the number of nodes that are common
	// between the new and the old committee
	nCommon12 := 1

	// The number of new added nodes. the new committee should have nCommon+nNew
	// nodes in totatl
	n2 := 2
	threshold2 := nCommon12 + n2
	minos2 := make([]mino.Mino, n2+nCommon12)
	dkgs2 := make([]dkg.DKG, n2+nCommon12)
	addrs2 := make([]mino.Address, n2+nCommon12)

	// The first nCommon nodes of  committee are the same as the first nCommon
	// nodes of the old committee
	for i := 0; i < nCommon12; i++ {
		minos2[i] = minos1[i]
		addrs2[i] = minos1[i].GetAddress()
	}

	pubkeys2 := make([]kyber.Point, len(minos2))

	// Defining the address of the new nodes.
	for i := 0; i < n2; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr new %d", i))
		minos2[i+nCommon12] = minogrpc
		addrs2[i+nCommon12] = minogrpc.GetAddress()
	}

	// Initializing the pedersen of the new nodes. the common nodes already have
	// a pedersen
	for i := 0; i < n2; i++ {
		dkg, pubkey := NewPedersen(minos2[i+nCommon12])
		dkgs2[i+nCommon12] = dkg
		pubkeys2[i+nCommon12] = pubkey
	}

	for i := 0; i < nCommon12; i++ {
		dkgs2[i] = dkgs1[i]
		pubkeys2[i] = pubkeys1[i]
	}

	// Initializing the actor of the new nodes. the common nodes already have an
	// actor
	actors2 := make([]dkg.Actor, n2+nCommon12)

	for i := 0; i < nCommon12; i++ {
		actors2[i] = actors1[i]
	}

	for i := 0; i < n2; i++ {
		actor, err := dkgs2[i+nCommon12].Listen()
		require.NoError(t, err)
		actors2[i+nCommon12] = actor
	}

	// Resharing the committee secret among the new committee
	fakeAuthority = NewAuthority(addrs2, pubkeys2)
	err = actors1[0].Reshare(fakeAuthority, threshold2)
	require.NoError(t, err, "Resharing was not successful")

	// Comparing the public key of the old and the new committee
	oldPubKey, err := actors1[0].GetPublicKey()
	require.NoError(t, err)

	for _, actorNew := range actors2 {
		newPubKey, err := actorNew.GetPublicKey()

		// The public key should remain the same
		require.NoError(t, err, "the public key should remain the same")
		newPubKey.Equal(oldPubKey)
		decrypted, err := actorNew.Decrypt(K, C)
		require.NoError(t, err, "decryption was not successful")
		require.Equal(t, message, decrypted, "the new committee should be able "+
			"to decrypt the messages encrypted by the old committee")
	}

	// Setting up the third dkg
	// 1 completely new node
	n3 := 1

	// This dkg committee shares 2 nodes with the second committee
	threshold3 := 2 + n3
	minos3 := make([]mino.Mino, 2+n3)
	dkgs3 := make([]dkg.DKG, 2+n3)
	addrs3 := make([]mino.Address, 2+n3)

	// Node with index 0 in the third dkg is a node from the second committee
	minos3[0] = minos2[n2-1]
	addrs3[0] = minos2[n2-1].GetAddress()

	// Node with index 1 in the third dkg is a node from both the first and the
	// second committee
	minos3[1] = minos2[0]
	addrs3[1] = minos2[0].GetAddress()

	pubkeys3 := make([]kyber.Point, len(minos3))

	// Defining the address of the new node.
	for i := 0; i < n3; i++ {
		minogrpc := minoch.MustCreate(minoManager, fmt.Sprintf("addr new new %d", i))
		minos3[i+2] = minogrpc
		addrs3[i+2] = minogrpc.GetAddress()
	}

	dkgs3[0] = dkgs2[n2-1]
	pubkeys3[0] = pubkeys2[n2-1]

	dkgs3[1] = dkgs2[0]
	pubkeys3[1] = pubkeys2[0]

	// Initializing the pedersen of the new node. the common nodes already have
	// a pedersen
	for i := 0; i < n3; i++ {
		dkg, pubkey := NewPedersen(minos3[i+2])
		dkgs3[i+2] = dkg
		pubkeys3[i+2] = pubkey
	}

	actors3 := make([]dkg.Actor, n3+2)

	actors3[0] = actors2[n2-1]
	actors3[1] = actors2[0]

	// Initializing the actor of the new nodes. the common nodes already have an
	// actor
	for i := 0; i < n3; i++ {
		actor, err := dkgs3[i+2].Listen()
		require.NoError(t, err)
		actors3[i+2] = actor
	}

	// Resharing the committee secret among the third committee
	fakeAuthority = NewAuthority(addrs3, pubkeys3)
	err = actors2[2].Reshare(fakeAuthority, threshold3)
	require.NoError(t, err, "Resharing was not successful")

	// Comparing the public key of the second and the third committee
	oldPubKey, err = actors2[1].GetPublicKey()
	require.NoError(t, err)

	for _, actorNew := range actors3 {
		newPubKey, err := actorNew.GetPublicKey()

		// The public key should remain the same
		require.NoError(t, err, "the public key should remain the same")
		newPubKey.Equal(oldPubKey)
		decrypted, err := actorNew.Decrypt(K, C)
		require.NoError(t, err, "decryption was not successful")
		require.Equal(t, message, decrypted, "the new committee should be able "+
			"to decrypt the messages encrypted by the old committee")
	}

}
