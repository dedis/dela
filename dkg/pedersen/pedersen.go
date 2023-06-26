package pedersen

import (
	"crypto/sha256"
	"runtime"
	"sync"
	"time"

	"go.dedis.ch/dela"

	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/random"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

// initDkgFirst message helping the developer to verify whether setup did occur
const initDkgFirst = "you must first initialize DKG. Did you call setup() first?"

// failedStreamCreation message indicating a stream creation failure
const failedStreamCreation = "failed to create stream: %v"

// unexpectedStreamStop message indicating that a stream stopped unexpectedly
const unexpectedStreamStop = "stream stopped unexpectedly: %v"

// unexpectedReply message indicating that a reply was not expected
const unexpectedReply = "got unexpected reply, expected %T but got: %T"

// suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

var (
	// protocolNameSetup denotes the value of the protocol span tag associated
	// with the `dkg-setup` protocol.
	protocolNameSetup = "dkg-setup"
	// protocolNameDecrypt denotes the value of the protocol span tag
	// associated with the `dkg-decrypt` protocol.
	protocolNameDecrypt = "dkg-decrypt"
	// protocolNameReencrypt denotes the value of the protocol span tag
	//// associated with the `dkg-reencrypt` protocol.
	protocolNameReencrypt = "dkg-reencrypt"
	// ProtocolNameResharing denotes the value of the protocol span tag
	// associated with the `dkg-resharing` protocol.
	protocolNameResharing = "dkg-resharing"
	// number of workers used to perform the encryption/decryption
	workerNum = runtime.NumCPU()
)

const (
	setupTimeout     = time.Minute * 50
	decryptTimeout   = time.Minute * 5
	resharingTimeout = time.Minute * 5
)

// Pedersen allows one to initialize a new DKG protocol.
//
// - implements dkg.DKG
type Pedersen struct {
	privKey kyber.Scalar
	mino    mino.Mino
	factory serde.Factory
}

// NewPedersen returns a new DKG Pedersen factory
func NewPedersen(m mino.Mino) (*Pedersen, kyber.Point) {
	factory := types.NewMessageFactory(m.GetAddressFactory())

	privkey := suite.Scalar().Pick(suite.RandomStream())
	pubkey := suite.Point().Mul(privkey, nil)

	return &Pedersen{
		privKey: privkey,
		mino:    m,
		factory: factory,
	}, pubkey
}

// Listen implements dkg.DKG. It must be called on each node that participates
// in the DKG. Creates the RPC.
func (s *Pedersen) Listen() (dkg.Actor, error) {
	h := NewHandler(s.privKey, s.mino.GetAddress())

	a := &Actor{
		rpc:      mino.MustCreateRPC(s.mino, "dkg", h, s.factory),
		factory:  s.factory,
		startRes: h.dkgInstance.getState(),
	}

	return a, nil
}

// Actor allows one to perform DKG operations like encrypt/decrypt a message
//
// Currently, a lot of the Actor code is dealing with low-level crypto.
// TODO: split (high-level) Actor functions and (low-level) DKG crypto. (#241)
//
// - implements dkg.Actor
type Actor struct {
	rpc      mino.RPC
	factory  serde.Factory
	startRes *state
}

// Setup implement dkg.Actor. It initializes the DKG.
func (a *Actor) Setup(coAuth crypto.CollectiveAuthority, threshold int) (kyber.Point, error) {

	if a.startRes.Done() {
		return nil, xerrors.Errorf("startRes is already done, only one setup call is allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameSetup)

	sender, receiver, err := a.rpc.Stream(ctx, coAuth)
	if err != nil {
		return nil, xerrors.Errorf("failed to stream: %v", err)
	}

	addrs := make([]mino.Address, 0, coAuth.Len())
	pubkeys := make([]kyber.Point, 0, coAuth.Len())

	addrIter := coAuth.AddressIterator()
	pubkeyIter := coAuth.PublicKeyIterator()

	for addrIter.HasNext() && pubkeyIter.HasNext() {
		addrs = append(addrs, addrIter.GetNext())

		pubkey := pubkeyIter.GetNext()
		edKey, ok := pubkey.(ed25519.PublicKey)
		if !ok {
			return nil, xerrors.Errorf("expected ed25519.PublicKey, got '%T'", pubkey)
		}

		pubkeys = append(pubkeys, edKey.GetPoint())
	}

	message := types.NewStart(threshold, addrs, pubkeys)

	errs := sender.Send(message, addrs...)
	err = <-errs
	if err != nil {
		return nil, xerrors.Errorf("failed to send start: %v", err)
	}

	dkgPubKeys := make([]kyber.Point, len(addrs))

	for i := 0; i < len(addrs); i++ {

		addr, msg, err := receiver.Recv(context.Background())
		if err != nil {
			return nil, xerrors.Errorf("got an error from '%s' while "+
				"receiving: %v", addr, err)
		}

		doneMsg, ok := msg.(types.StartDone)
		if !ok {
			return nil, xerrors.Errorf("expected to receive a Done message, but "+
				"go the following: %T", msg)
		}

		dela.Logger.Info().Msgf("node %q done", addr.String())

		dkgPubKeys[i] = doneMsg.GetPublicKey()

		// this is a simple check that every node sends back the same DKG pub
		// key.
		// TODO: handle the situation where a pub key is not the same
		if i != 0 && !dkgPubKeys[i-1].Equal(doneMsg.GetPublicKey()) {
			return nil, xerrors.Errorf("the public keys does not match: %v", dkgPubKeys)
		}
	}

	return dkgPubKeys[0], nil
}

// GetPublicKey implements dkg.Actor
func (a *Actor) GetPublicKey() (kyber.Point, error) {
	if !a.startRes.Done() {
		return nil, xerrors.Errorf("DKG has not been initialized")
	}

	return a.startRes.getDistKey(), nil
}

// Encrypt implements dkg.Actor. It uses the DKG public key to encrypt a
// message, and returns a random, ephemeral part K
// and the cipher as an array of Kyber points
func (a *Actor) Encrypt(msg []byte) (kyber.Point, []kyber.Point, error) {

	if !a.startRes.Done() {
		return nil, nil, xerrors.Errorf(initDkgFirst)
	}

	pubK, err := a.GetPublicKey()
	if err != nil {
		dela.Logger.Error().Msgf("Cannot encrypt: %v", err.Error())
	}

	// ElGamal-encrypt the point to produce ciphertext (K,C).
	r := suite.Scalar().Pick(suite.RandomStream())

	K := suite.Point().Mul(r, nil)
	dela.Logger.Debug().Msgf("K: %v", K.String())

	C := suite.Point().Mul(r, pubK)
	dela.Logger.Debug().Msgf("C: %v", C)

	// S: ephemeral DH shared secret
	S := suite.Point().Mul(r, pubK)
	dela.Logger.Debug().Msgf("S: %v", S.String())

	Cs := make([]kyber.Point, 0, 16)
	for len(msg) > 0 {
		kp := suite.Point().Embed(msg, suite.RandomStream())
		dela.Logger.Debug().Msgf("kp: %v", kp.String())

		// message blinded with secret
		c := suite.Point().Add(C, kp)
		dela.Logger.Debug().Msgf("c: %v", c)

		Cs = append(Cs, c)
		dela.Logger.Debug().Msgf("Cs: %v", Cs)

		msg = msg[min(len(msg), kp.EmbedLen()):]
	}

	return K, Cs, nil
}

// Decrypt implements dkg.Actor. It gets the private shares of the nodes and
// decrypt the  message.
func (a *Actor) Decrypt(K kyber.Point, Cs []kyber.Point) ([]byte, error) {

	if !a.startRes.Done() {
		return nil, xerrors.Errorf(initDkgFirst)
	}

	ctx, cancel := context.WithTimeout(context.Background(), decryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameDecrypt)

	players := mino.NewAddresses(a.startRes.getParticipants()...)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, xerrors.Errorf(failedStreamCreation, err)
	}

	iterator := players.AddressIterator()
	addrs := make([]mino.Address, 0, players.Len())

	for iterator.HasNext() {
		addrs = append(addrs, iterator.GetNext())
	}

	var decryptedMessage []byte

	for _, C := range Cs {
		message := types.NewDecryptRequest(K, C)

		err = <-sender.Send(message, addrs...)
		if err != nil {
			return nil, xerrors.Errorf("failed to send decrypt request: %v", err)
		}

		pubShares := make([]*share.PubShare, len(addrs))

		for i := 0; i < len(addrs); i++ {
			src, message, err := receiver.Recv(ctx)
			if err != nil {
				return []byte{}, xerrors.Errorf(unexpectedStreamStop, err)
			}

			dela.Logger.Debug().Msgf("Received a decryption reply from %v", src)

			decryptReply, ok := message.(types.DecryptReply)
			if !ok {
				return []byte{}, xerrors.Errorf(unexpectedReply, decryptReply, message)
			}

			pubShares[i] = &share.PubShare{
				I: int(decryptReply.I),
				V: decryptReply.V,
			}
		}

		res, err := share.RecoverCommit(suite, pubShares, len(addrs), len(addrs))
		if err != nil {
			return []byte{}, xerrors.Errorf("failed to recover commit: %v", err)
		}

		decrypted, err := res.Data()
		if err != nil {
			return []byte{}, xerrors.Errorf("failed to get embedded data: %v", err)
		}

		decryptedMessage = append(decryptedMessage, decrypted...)
	}
	dela.Logger.Info().Msgf("Decrypted message: %v", decryptedMessage)

	return decryptedMessage, nil
}

// VerifiableEncrypt implements dkg.Actor. It uses the DKG public key to encrypt
// a message and provide a zero knowledge proof that the encryption is done by
// this person.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 1
func (a *Actor) VerifiableEncrypt(message []byte, GBar kyber.Point) (types.Ciphertext,
	[]byte, error) {

	if !a.startRes.Done() {
		return types.Ciphertext{}, nil, xerrors.Errorf("you must first initialize " +
			"DKG. Did you call setup() first?")
	}

	// Embed the message (or as much of it as will fit) into a curve point.
	M := suite.Point().Embed(message, random.New())

	max := suite.Point().EmbedLen()
	if max > len(message) {
		max = len(message)
	}

	remainder := message[max:]

	// ElGamal-encrypt the point to produce ciphertext (localK,localC).
	localk := suite.Scalar().Pick(random.New())                  // ephemeral private key
	localK := suite.Point().Mul(localk, nil)                     // ephemeral DH public key
	localS := suite.Point().Mul(localk, a.startRes.getDistKey()) // ephemeral DH shared secret
	localC := localS.Add(localS, M)                              // message blinded with secret

	// producing the zero knowledge proof
	UBar := suite.Point().Mul(localk, GBar)
	s := suite.Scalar().Pick(random.New())
	W := suite.Point().Mul(s, nil)
	WBar := suite.Point().Mul(s, GBar)

	hash := sha256.New()
	localC.MarshalTo(hash)
	localK.MarshalTo(hash)
	UBar.MarshalTo(hash)
	W.MarshalTo(hash)
	WBar.MarshalTo(hash)

	E := suite.Scalar().SetBytes(hash.Sum(nil))
	F := suite.Scalar().Add(s, suite.Scalar().Mul(E, localk))

	ciphertext := types.Ciphertext{
		K:    localK,
		C:    localC,
		UBar: UBar,
		E:    E,
		F:    F,
		GBar: GBar,
	}

	return ciphertext, remainder, nil
}

// VerifiableDecrypt implements dkg.Actor. It does as Decrypt() but in addition
// it checks whether the decryption proofs are valid.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func (a *Actor) VerifiableDecrypt(ciphertexts []types.Ciphertext) ([][]byte, error) {

	if !a.startRes.Done() {
		return nil, xerrors.Errorf(initDkgFirst)
	}

	players := mino.NewAddresses(a.startRes.getParticipants()...)

	ctx, cancel := context.WithTimeout(context.Background(), decryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameDecrypt)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, xerrors.Errorf(failedStreamCreation, err)
	}

	players = mino.NewAddresses(a.startRes.getParticipants()...)
	iterator := players.AddressIterator()

	addrs := make([]mino.Address, 0, players.Len())
	for iterator.HasNext() {
		addrs = append(addrs, iterator.GetNext())
	}

	batchsize := len(ciphertexts)

	message := types.NewVerifiableDecryptRequest(ciphertexts)
	// sending the decrypt request to the nodes
	err = <-sender.Send(message, addrs...)
	if err != nil {
		return nil, xerrors.Errorf("failed to send verifiable decrypt request: %v", err)
	}

	responses := make([]types.VerifiableDecryptReply, len(addrs))

	// receive decrypt reply from the nodes
	for i := range addrs {
		from, message, err := receiver.Recv(ctx)
		if err != nil {
			return nil, xerrors.Errorf(unexpectedStreamStop, err)
		}

		dela.Logger.Debug().Msgf("received share from %v\n", from)

		shareAndProof, ok := message.(types.VerifiableDecryptReply)
		if !ok {
			return nil, xerrors.Errorf(unexpectedReply, shareAndProof, message)
		}

		responses[i] = shareAndProof
	}

	// the final decrypted message
	decryptedMessage := make([][]byte, batchsize)

	var wgBatchReply sync.WaitGroup
	jobChan := make(chan int)

	go func() {
		for i := 0; i < batchsize; i++ {
			jobChan <- i
		}

		close(jobChan)
	}()

	if batchsize < workerNum {
		workerNum = batchsize
	}

	worker := newWorker(len(addrs), decryptedMessage, responses, ciphertexts)

	for i := 0; i < workerNum; i++ {
		wgBatchReply.Add(1)

		go func() {
			defer wgBatchReply.Done()
			for j := range jobChan {
				err := worker.work(j)
				if err != nil {
					dela.Logger.Err(err).Msgf("error in a worker")
				}
			}
		}()
	}

	wgBatchReply.Wait()

	return decryptedMessage, nil
}

func newWorker(numParticipants int, decryptedMessage [][]byte,
	responses []types.VerifiableDecryptReply, ciphertexts []types.Ciphertext) worker {

	return worker{
		numParticipants:  numParticipants,
		decryptedMessage: decryptedMessage,
		responses:        responses,
		ciphertexts:      ciphertexts,
	}
}

// worker contains the data needed by a worker to perform the verifiable
// decryption job. All its fields must be read-only, except the
// decryptedMessage, which can be written at a provided jobIndex.
type worker struct {
	numParticipants  int
	decryptedMessage [][]byte
	ciphertexts      []types.Ciphertext
	responses        []types.VerifiableDecryptReply
}

func (w worker) work(jobIndex int) error {
	pubShares := make([]*share.PubShare, w.numParticipants)

	for k, response := range w.responses {
		resp := response.GetShareAndProof()[jobIndex]

		err := checkDecryptionProof(resp, w.ciphertexts[jobIndex].K)
		if err != nil {
			return xerrors.Errorf("failed to check the decryption proof: %v", err)
		}

		pubShares[k] = &share.PubShare{
			I: int(resp.I),
			V: resp.V,
		}
	}

	res, err := share.RecoverCommit(suite, pubShares, w.numParticipants, w.numParticipants)
	if err != nil {
		return xerrors.Errorf("failed to recover the commit: %v", err)
	}

	w.decryptedMessage[jobIndex], err = res.Data()
	if err != nil {
		return xerrors.Errorf("failed to get embedded data : %v", err)
	}

	return nil
}

// Reshare implements dkg.Actor. It recreates the DKG with an updated list of
// participants.
func (a *Actor) Reshare(coAuth crypto.CollectiveAuthority, thresholdNew int) error {
	if !a.startRes.Done() {
		return xerrors.Errorf(initDkgFirst)
	}

	addrsNew := make([]mino.Address, 0, coAuth.Len())
	pubkeysNew := make([]kyber.Point, 0, coAuth.Len())

	addrIter := coAuth.AddressIterator()
	pubkeyIter := coAuth.PublicKeyIterator()

	for addrIter.HasNext() && pubkeyIter.HasNext() {
		addrsNew = append(addrsNew, addrIter.GetNext())

		pubkey := pubkeyIter.GetNext()

		edKey, ok := pubkey.(ed25519.PublicKey)
		if !ok {
			return xerrors.Errorf("expected ed25519.PublicKey, got '%T'", pubkey)
		}

		pubkeysNew = append(pubkeysNew, edKey.GetPoint())
	}

	// Get the union of the new members and the old members
	addrsAll := union(a.startRes.getParticipants(), addrsNew)
	players := mino.NewAddresses(addrsAll...)

	ctx, cancel := context.WithTimeout(context.Background(), resharingTimeout)
	defer cancel()

	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameResharing)

	dela.Logger.Info().Msgf("resharing with the following participants: %v", addrsAll)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return xerrors.Errorf(failedStreamCreation, err)
	}

	thresholdOld := a.startRes.getThreshold()
	pubkeysOld := a.startRes.getPublicKeys()

	// We don't need to send the old threshold or old public keys to the old or
	// common nodes
	reshare := types.NewStartResharing(thresholdNew, 0, addrsNew, nil, pubkeysNew, nil)

	dela.Logger.Info().Msgf("resharing to old participants: %v",
		a.startRes.getParticipants())

	// Send the resharing request to the old and common nodes
	err = <-sender.Send(reshare, a.startRes.getParticipants()...)
	if err != nil {
		return xerrors.Errorf("failed to send resharing request: %v", err)
	}

	// First find the set of new nodes that are not common between the old and
	// new committee
	newParticipants := difference(addrsNew, a.startRes.getParticipants())

	// Then create a resharing request message for them. We should send the old
	// threshold and old public keys to them
	reshare = types.NewStartResharing(thresholdNew, thresholdOld, addrsNew,
		a.startRes.getParticipants(), pubkeysNew, pubkeysOld)

	dela.Logger.Info().Msgf("resharing to new participants: %v", newParticipants)

	// Send the resharing request to the new but not common nodes
	err = <-sender.Send(reshare, newParticipants...)
	if err != nil {
		return xerrors.Errorf("failed to send resharing request: %v", err)
	}

	dkgPubKeys := make([]kyber.Point, len(addrsAll))

	// Wait for receiving the response from the new nodes
	for i := 0; i < len(addrsAll); i++ {
		src, msg, err := receiver.Recv(ctx)
		if err != nil {
			return xerrors.Errorf(unexpectedStreamStop, err)
		}

		doneMsg, ok := msg.(types.StartDone)
		if !ok {
			return xerrors.Errorf("expected to receive a Done message, but "+
				"got the following: %T, from %s", msg, src.String())
		}

		dkgPubKeys[i] = doneMsg.GetPublicKey()

		dela.Logger.Debug().Str("from", src.String()).Msgf("received a done reply")

		// This is a simple check that every node sends back the same DKG pub
		// key.
		// TODO: handle the situation where a pub key is not the same
		if i != 0 && !dkgPubKeys[i-1].Equal(doneMsg.GetPublicKey()) {
			return xerrors.Errorf("the public keys does not match: %v", dkgPubKeys)
		}
	}

	dela.Logger.Info().Msgf("resharing done")

	return nil
}

// difference performs "el1 difference el2", i.e. it extracts all members of el1
// that are not present in el2.
func difference(el1 []mino.Address, el2 []mino.Address) []mino.Address {
	var result []mino.Address

	for _, addr1 := range el1 {
		exist := false
		for _, addr2 := range el2 {
			if addr1.Equal(addr2) {
				exist = true
				break
			}
		}

		if !exist {
			result = append(result, addr1)
		}
	}

	return result
}

// union performs a union of el1 and el2.
func union(el1 []mino.Address, el2 []mino.Address) []mino.Address {
	addrsAll := el1

	for _, other := range el2 {
		exist := false
		for _, addr := range el1 {
			if addr.Equal(other) {
				exist = true
				break
			}
		}
		if !exist {
			addrsAll = append(addrsAll, other)
		}
	}

	return addrsAll
}
