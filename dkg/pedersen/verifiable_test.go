package pedersen

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"

	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/router/tree"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/xof/keccak"
)

func init() {
	rand.Seed(0)
}

func Test_VerifiableEncrypt_NotInit(t *testing.T) {
	a := Actor{
		startRes: &state{},
	}
	_, _, err := a.VerifiableEncrypt(nil, nil)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")
}

func Test_VerifiableEncrypt_ShostMsg(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
	}
	_, reminder, err := a.VerifiableEncrypt(make([]byte, 28), nil)
	require.NoError(t, err)
	require.Len(t, reminder, 0)
}

func Test_VerifiableEncDec_minoch(t *testing.T) {
	// setup up the dkg
	n := 18
	threshold := n
	batchSize := 10

	minos := make([]mino.Mino, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	// Create GBar. We need a generator in order to follow the encryption and
	// decryption protocol of https://arxiv.org/pdf/2205.08529.pdf / we take an
	// agreed data among the participants and embed it as a point. The result is
	// the generator that we are seeking.
	agreedData := make([]byte, 32)
	_, err := rand.Read(agreedData)
	require.NoError(t, err)
	GBar := suite.Point().Embed(agreedData, keccak.New(agreedData))

	minoManager := minoch.NewManager()

	for i := 0; i < n; i++ {
		minoch := minoch.MustCreate(minoManager, fmt.Sprintf("addr %d", i))
		minos[i] = minoch
		addrs[i] = minoch.GetAddress()
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

	t.Log("setting up the dkg ...")

	_, err = actors[0].Setup(fakeAuthority, threshold)
	require.NoError(t, err)

	t.Log("generating the message and encrypting it ...")

	// generating random messages in batch and encrypt them
	keys := make([][29]byte, batchSize)

	var ciphertexts []types.Ciphertext
	for i := 0; i < batchSize; i++ {
		_, err = rand.Read(keys[i][:])
		require.NoError(t, err)

		ciphertext, remainder, err := actors[0].VerifiableEncrypt(keys[i][:], GBar)
		require.NoError(t, err)
		require.Len(t, remainder, 0)

		ciphertexts = append(ciphertexts, ciphertext)
	}

	t.Log("decrypting the ciphertext ...")

	decrypted, err := actors[0].VerifiableDecrypt(ciphertexts)
	require.NoError(t, err)
	for i := 0; i < batchSize; i++ {
		require.Equal(t, keys[i][:], decrypted[i])
	}
}

func Test_VerifiableEncDec_minogrpc(t *testing.T) {
	// we want to time the decryption for different batch sizes with different
	// number of nodes

	numWorkersSlice := []int{8}
	batchSizeSlice := []int{32}

	// setting up the dkg
	n := 18
	threshold := n

	minos := make([]mino.Mino, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	// Create GBar. We need a generator in order to follow the encryption and
	// decryption protocol of https://arxiv.org/pdf/2205.08529.pdf. We take an
	// agreed data among the participants and embed it as a point. The result is
	// the generator that we are seeking.
	agreedData := make([]byte, 32)
	_, err := rand.Read(agreedData)
	require.NoError(t, err)
	GBar := suite.Point().Embed(agreedData, keccak.New(agreedData))

	t.Log("initiating the dkg nodes ...")

	for i := 0; i < n; i++ {
		addr := minogrpc.ParseAddress("127.0.0.1", 0)

		minogrpc, err := minogrpc.NewMinogrpc(addr, nil, tree.NewRouter(minogrpc.NewAddressFactory()))
		require.NoError(t, err)

		defer minogrpc.GracefulStop()

		minos[i] = minogrpc
		addrs[i] = minogrpc.GetAddress()
	}

	pubkeys := make([]kyber.Point, len(minos))

	for i, mino := range minos {
		for _, m := range minos {
			mino.(*minogrpc.Minogrpc).GetCertificateStore().Store(m.GetAddress(),
				m.(*minogrpc.Minogrpc).GetCertificateChain())
		}
		dkg, pubkey := NewPedersen(mino.(*minogrpc.Minogrpc))
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

	t.Log("setting up the dkg ...")

	start := time.Now()
	_, err = actors[0].Setup(fakeAuthority, threshold)
	require.NoError(t, err)
	setupTime := time.Since(start)

	// generating random messages in batch and encrypt them
	for i, batchSize := range batchSizeSlice {
		t.Logf("=== starting the process with batch size = %d === \n", batchSize)

		workerNum = numWorkersSlice[i]

		msg := make([][29]byte, batchSize)
		var ciphertexts []types.Ciphertext
		for i := 0; i < batchSize; i++ {
			_, err = rand.Read(msg[i][:])
			require.NoError(t, err)

			ciphertext, remainder, err := actors[0].VerifiableEncrypt(msg[i][:], GBar)
			require.NoError(t, err)
			require.Len(t, remainder, 0)

			ciphertexts = append(ciphertexts, ciphertext)
		}

		t.Log("decrypting the batch ...")

		start = time.Now()
		decrypted, err := actors[0].VerifiableDecrypt(ciphertexts)
		decryptionTime := time.Since(start)
		require.NoError(t, err)

		for i := 0; i < batchSize; i++ {
			require.Equal(t, msg[i][:], decrypted[i])
		}

		t.Logf("n=%d, batchSize=%d, workerNum=%d, decryption time=%s, "+
			"throughput=%v[tx/s], dkg setup time=%s", n, batchSize, workerNum,
			decryptionTime, float64(batchSize)/decryptionTime.Seconds(), setupTime)
	}
}

func Test_VerifiableDecrypt_NotStarted(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: initial},
	}

	_, err := a.VerifiableDecrypt(nil)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")
}

func Test_VerifiableDecrypt_BadRPC(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
		rpc:      fake.NewBadRPC(),
	}

	_, err := a.VerifiableDecrypt(nil)
	require.EqualError(t, err, fake.Err("failed to create stream"))
}

func Test_VerifiableDecrypt_BadSender(t *testing.T) {
	a := Actor{
		startRes: &state{dkgState: certified},
		rpc:      fake.NewStreamRPC(nil, fake.NewBadSender()),
	}

	_, err := a.VerifiableDecrypt(nil)
	require.EqualError(t, err, fake.Err("failed to send verifiable decrypt request"))
}

func Test_VerifiableDecrypt_BadReceiver(t *testing.T) {
	a := Actor{
		startRes: &state{
			dkgState:     certified,
			participants: []mino.Address{fake.NewAddress(0)},
		},
		rpc: fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{}),
	}

	_, err := a.VerifiableDecrypt(nil)
	require.EqualError(t, err, fake.Err("stream stopped unexpectedly"))
}

func Test_VerifiableDecrypt_BadReply(t *testing.T) {
	reply := fake.NewRecvMsg(fake.NewAddress(0), fake.Message{})

	a := Actor{
		startRes: &state{
			dkgState:     certified,
			participants: []mino.Address{fake.NewAddress(0)},
		},
		rpc: fake.NewStreamRPC(&fake.Receiver{
			Msgs: []fake.ReceiverMessage{reply},
		}, fake.Sender{}),
	}

	_, err := a.VerifiableDecrypt(nil)
	require.EqualError(t, err, "got unexpected reply, expected types.VerifiableDecryptReply but got: fake.Message")
}
