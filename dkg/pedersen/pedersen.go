package pedersen

import "C"
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
	// associated with the `dkg-reencrypt` protocol.
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
	reencryptTimeout = time.Minute * 5
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
func (a *Actor) Setup(co crypto.CollectiveAuthority, threshold int) (kyber.Point, error) {

	if a.startRes.Done() {
		return nil, xerrors.Errorf("startRes is already done, only one setup call is allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameSetup)

	sender, receiver, err := a.rpc.Stream(ctx, co)
	if err != nil {
		return nil, xerrors.Errorf("failed to stream: %v", err)
	}

	addrs := make([]mino.Address, 0, co.Len())
	pubkeys := make([]kyber.Point, 0, co.Len())

	addrIter := co.AddressIterator()
	pubkeyIter := co.PublicKeyIterator()

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
// message.
func (a *Actor) Encrypt(message []byte) (K, C kyber.Point, remainder []byte,
	err error) {

	if !a.startRes.Done() {
		return nil, nil, nil, xerrors.Errorf(initDkgFirst)
	}

	// Embed the message (or as much of it as will fit) into a curve point.
	M := suite.Point().Embed(message, random.New())
	max := suite.Point().EmbedLen()
	if max > len(message) {
		max = len(message)
	}
	remainder = message[max:]
	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := suite.Scalar().Pick(random.New())             // ephemeral private key
	K = suite.Point().Mul(k, nil)                      // ephemeral DH public key
	S := suite.Point().Mul(k, a.startRes.getDistKey()) // ephemeral DH shared secret
	C = S.Add(S, M)                                    // message blinded with secret

	return K, C, remainder, nil
}

// Reencrypt implements dkg.Actor. It uses the given public key
// to reencrypt an encrypted message.
func (a *Actor) Reencrypt(k, c, pk kyber.Point) (K, C kyber.Point, remainder []byte, err error) {
	if !a.startRes.Done() {
		return nil, nil, []byte{}, xerrors.Errorf(initDkgFirst)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reencryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameReencrypt)

	players := mino.NewAddresses(a.startRes.getParticipants()...)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, nil, []byte{}, xerrors.Errorf(failedStreamCreation, err)
	}

	iterator := players.AddressIterator()
	addrs := make([]mino.Address, 0, players.Len())

	for iterator.HasNext() {
		addrs = append(addrs, iterator.GetNext())
	}

	//TODO: how to calculate (k, pk) in place of (U, Xc) ?
	// seems correct, as in:
	//	type OCS struct {
	//	*onet.TreeNodeInstance
	//	Shared    *dkgprotocol.SharedSecret // Shared represents the private key
	//	Poly      *share.PubPoly            // Represents all public keys
	//	U         kyber.Point               // U is the encrypted secret
	//	Xc        kyber.Point               // The client's public key
	//	Threshold int                       // How many replies are needed
	//	to re-create the secret
	message := types.NewReencryptRequest(k, pk)

	err = <-sender.Send(message, addrs...)
	if err != nil {
		return nil, nil, []byte{}, xerrors.Errorf("failed to send decrypt request: %v", err)
	}

	pubShares := make([]*share.PubShare, len(addrs))
	U := k
	Xc := pk
	pubShares[0] = a.getUI(k, pk) // TODO: verify that this getUI(U, Xc) call is correct

	for i := 0; i < len(addrs); i++ {
		src, message, err := receiver.Recv(ctx)
		if err != nil && i < a.startRes.threshold {
			return nil, nil, []byte{}, xerrors.Errorf(unexpectedStreamStop, err)
		}

		dela.Logger.Debug().Msgf("Received a reencryption reply from %v", src)

		r, ok := message.(types.ReencryptReply)
		if !ok {
			return nil, nil, []byte{}, xerrors.Errorf("got unexpected reply, expected "+
				"%T but got: %T", r, message)
		}

		// Verify proofs
		ufi := suite.Point().Mul(r.Fi, suite.Point().Add(U, Xc))
		uiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), r.Ui.V)
		uiHat := suite.Point().Add(ufi, uiei)

		gfi := suite.Point().Mul(r.Fi, nil)
		gxi := a.startRes.poly.Eval(r.Ui.I).V //TODO: Poly *share.PubPoly represents all public keys
		hiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), gxi)
		hiHat := suite.Point().Add(gfi, hiei)
		hash := sha256.New()
		r.Ui.V.MarshalTo(hash)
		uiHat.MarshalTo(hash)
		hiHat.MarshalTo(hash)

		e := suite.Scalar().SetBytes(hash.Sum(nil))
		if e.Equal(r.Ei) {
			pubShares[r.Ui.I] = r.Ui
		} else {
			dela.Logger.Warn().Msgf("Received invalid share %v from node", r.Ui.I)
		}
	}

	res, err := share.RecoverCommit(suite, pubShares, len(addrs), len(addrs))
	if err != nil {
		return nil, nil, []byte{}, xerrors.Errorf("failed to recover commit: %v", err)
	}

	decryptedMessage, err := res.Data()
	if err != nil {
		return nil, nil, []byte{}, xerrors.Errorf("failed to get embedded data: %v", err)
	}

	dela.Logger.Info().Msgf("Decrypted message: %v", decryptedMessage)

	//	return new encrypted message, nil
	return K, C, remainder, nil
}

// TODO: where to get this UI ?
func (a *Actor) getUI(U, Xc kyber.Point) *share.PubShare {
	v := suite.Point().Mul(suite.Scalar().Zero(), U)
	v.Add(v, suite.Point().Mul(suite.Scalar().Zero(), Xc))
	return &share.PubShare{
		I: 0,
		V: v,
	}
}

// VerifiableEncrypt implements dkg.Actor. It uses the DKG public key to encrypt
// a message and provide a zero knowledge proof that the encryption is done by
// this person.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 1
func (a *Actor) VerifiableEncrypt(message []byte, GBar kyber.Point) (ciphertext types.Ciphertext,
	remainder []byte, err error) {

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

	remainder = message[max:]

	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := suite.Scalar().Pick(random.New())             // ephemeral private key
	K := suite.Point().Mul(k, nil)                     // ephemeral DH public key
	S := suite.Point().Mul(k, a.startRes.getDistKey()) // ephemeral DH shared secret
	C := S.Add(S, M)                                   // message blinded with secret

	// producing the zero knowledge proof
	UBar := suite.Point().Mul(k, GBar)
	s := suite.Scalar().Pick(random.New())
	W := suite.Point().Mul(s, nil)
	WBar := suite.Point().Mul(s, GBar)

	hash := sha256.New()
	C.MarshalTo(hash)
	K.MarshalTo(hash)
	UBar.MarshalTo(hash)
	W.MarshalTo(hash)
	WBar.MarshalTo(hash)

	E := suite.Scalar().SetBytes(hash.Sum(nil))
	F := suite.Scalar().Add(s, suite.Scalar().Mul(E, k))

	ciphertext = types.Ciphertext{
		K:    K,
		C:    C,
		UBar: UBar,
		E:    E,
		F:    F,
		GBar: GBar,
	}

	return ciphertext, remainder, nil
}

// Decrypt implements dkg.Actor. It gets the private shares of the nodes and
// decrypt the  message.
func (a *Actor) Decrypt(K, C kyber.Point) ([]byte, error) {

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
			return []byte{}, xerrors.Errorf("got unexpected reply, expected "+
				"%T but got: %T", decryptReply, message)
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

	decryptedMessage, err := res.Data()
	if err != nil {
		return []byte{}, xerrors.Errorf("failed to get embedded data: %v", err)
	}

	dela.Logger.Info().Msgf("Decrypted message: %v", decryptedMessage)

	return decryptedMessage, nil
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
			return nil, xerrors.Errorf("got unexpected reply, expected "+
				"%T but got: %T", shareAndProof, message)
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

func (a *Actor) VerifiableReencrypt(cipher []types.Ciphertext, pubk kyber.Point) (ciphertext []types.Ciphertext, remainder []byte, err error) {
	if !a.startRes.Done() {
		return []types.Ciphertext{}, nil, xerrors.Errorf(initDkgFirst)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reencryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameReencrypt)

	players := mino.NewAddresses(a.startRes.getParticipants()...)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return []types.Ciphertext{}, nil, xerrors.Errorf(failedStreamCreation, err)
	}

	iterator := players.AddressIterator()

	addrs := make([]mino.Address, 0, players.Len())
	for iterator.HasNext() {
		addrs = append(addrs, iterator.GetNext())
	}

	//TODO: how to calculate (k, pk) in place of (U, Xc) ?
	// copied from cothority/calypso/protocol/ocs.go:
	//	type OCS struct {
	//	*onet.TreeNodeInstance
	//	Shared    *dkgprotocol.SharedSecret // Shared represents the private key
	//	Poly      *share.PubPoly            // Represents all public keys
	//	U         kyber.Point               // U is the encrypted secret
	//	Xc        kyber.Point               // The client's public key
	//	Threshold int                       // How many replies are needed
	//	to re-create the secret

	//TODO is the following correct ???
	U := cipher[0].K
	Xc := pubk

	batchsize := len(cipher)

	message := types.NewReencryptRequest(U, Xc)

	err = <-sender.Send(message, addrs...)
	if err != nil {
		return []types.Ciphertext{}, nil, xerrors.Errorf("failed to send decrypt request: %v", err)
	}

	pubShares := make([]*share.PubShare, len(addrs))
	pubShares[0] = a.getUI(U, Xc) // TODO: verify that this getUI(U, Xc) call is correct

	for i := 0; i < len(addrs); i++ {
		src, message, err := receiver.Recv(ctx)
		if err != nil && i < a.startRes.threshold {
			return []types.Ciphertext{}, nil, xerrors.Errorf(unexpectedStreamStop, err)
		}

		dela.Logger.Debug().Msgf("Received a reencryption reply from %v", src)

		r, ok := message.(types.ReencryptReply)
		if !ok {
			return []types.Ciphertext{}, nil, xerrors.Errorf("got unexpected reply, expected "+
				"%T but got: %T", r, message)
		}

		// Verify proofs
		ufi := suite.Point().Mul(r.Fi, suite.Point().Add(U, Xc))
		uiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), r.Ui.V)
		uiHat := suite.Point().Add(ufi, uiei)

		gfi := suite.Point().Mul(r.Fi, nil)
		gxi := a.startRes.poly.Eval(r.Ui.I).V //TODO: Poly *share.PubPoly represents all public keys
		hiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), gxi)
		hiHat := suite.Point().Add(gfi, hiei)
		hash := sha256.New()
		r.Ui.V.MarshalTo(hash)
		uiHat.MarshalTo(hash)
		hiHat.MarshalTo(hash)

		e := suite.Scalar().SetBytes(hash.Sum(nil))
		if e.Equal(r.Ei) {
			pubShares[r.Ui.I] = r.Ui
		} else {
			dela.Logger.Warn().Msgf("Received invalid share %v from node", r.Ui.I)
		}
	}

	res, err := share.RecoverCommit(suite, pubShares, len(addrs), len(addrs))
	if err != nil {
		return []types.Ciphertext{}, nil, xerrors.Errorf("failed to recover commit: %v", err)
	}

	decryptedMessage, err := res.Data()
	if err != nil {
		return []types.Ciphertext{}, nil, xerrors.Errorf("failed to get embedded data: %v", err)
	}

	dela.Logger.Info().Msgf("Decrypted message: %v", decryptedMessage)

	ciphertext = []types.Ciphertext{
		K:    K,
		C:    C,
		UBar: UBar,
		E:    E,
		F:    F,
		GBar: GBar,
	}

	return ciphertext, remainder, nil
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
func (a *Actor) Reshare(co crypto.CollectiveAuthority, thresholdNew int) error {
	if !a.startRes.Done() {
		return xerrors.Errorf(initDkgFirst)
	}

	addrsNew := make([]mino.Address, 0, co.Len())
	pubkeysNew := make([]kyber.Point, 0, co.Len())

	addrIter := co.AddressIterator()
	pubkeyIter := co.PublicKeyIterator()

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
