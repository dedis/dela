package pedersen

import (
	"crypto/sha256"
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

// suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

var (
	// protocolNameSetup denotes the value of the protocol span tag associated
	// with the `dkg-setup` protocol.
	protocolNameSetup = "dkg-setup"
	// protocolNameDecrypt denotes the value of the protocol span tag
	// associated with the `dkg-decrypt` protocol.
	protocolNameDecrypt = "dkg-decrypt"
	// ProtocolNameResharing denotes the value of the protocol span tag
	// associated with the `dkg-resharing` protocol.
	protocolNameResharing = "dkg-resharing"
)

const (
	setupTimeout     = time.Second * 300
	decryptTimeout   = time.Second * 10000
	resharingTimeout = time.Second * 300
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
		startRes: h.startRes,
	}

	return a, nil
}

// Actor allows one to perform DKG operations like encrypt/decrypt a message
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

	return a.startRes.GetDistKey(), nil
}

// Encrypt implements dkg.Actor. It uses the DKG public key to encrypt a
// message.
func (a *Actor) Encrypt(message []byte) (K, C kyber.Point, remainder []byte,
	err error) {

	if !a.startRes.Done() {
		return nil, nil, nil, xerrors.Errorf("you must first initialize DKG. " +
			"Did you call setup() first?")
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
	S := suite.Point().Mul(k, a.startRes.GetDistKey()) // ephemeral DH shared secret
	C = S.Add(S, M)                                    // message blinded with secret

	return K, C, remainder, nil
}

// Encrypt implements dkg.Actor. It uses the DKG public key to encrypt a
// message and provide a zero knowledge proof that the encryption is done by this person
// a discription can be found in https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / ster 1: write transaction
func (a *Actor) VerifiableEncrypt(message []byte, GBar kyber.Point) (ciphertext types.Ciphertext, remainder []byte,
	err error) {

	if !a.startRes.Done() {
		return types.Ciphertext{}, nil, xerrors.Errorf("you must first initialize DKG. " +
			"Did you call setup() first?")
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
	S := suite.Point().Mul(k, a.startRes.GetDistKey()) // ephemeral DH shared secret
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
// TODO: perform a re-encryption instead of gathering the private shares, which
// should never happen.
func (a *Actor) Decrypt(K, C kyber.Point) ([]byte, error) {

	if !a.startRes.Done() {
		return nil, xerrors.Errorf("you must first initialize DKG. " +
			"Did you call setup() first?")
	}

	players := mino.NewAddresses(a.startRes.GetParticipants()...)

	ctx, cancel := context.WithTimeout(context.Background(), decryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameDecrypt)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, xerrors.Errorf("failed to create stream: %v", err)
	}

	players = mino.NewAddresses(a.startRes.GetParticipants()...)
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
			return []byte{}, xerrors.Errorf("stream stopped unexpectedly: %v", err)
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
		return []byte{}, xerrors.Errorf("failed to get embeded data: %v", err)
	}

	dela.Logger.Info().Msgf("Decrypted message: %v", decryptedMessage)

	return decryptedMessage, nil
}

// VerifiableDecrypt implements dkg.Actor. This function gets a batch of ciphertext and dends it to the nodes and then
// receives the private shares of the nodes and decrypt the message. in addition it checks whether the decryption proofs are valid and returns the
// a discription can be found in https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / ster 3: key reconstruction
// TODO: perform a re-encryption instead of gathering the private shares, which
// should never happen.
func (a *Actor) VerifiableDecrypt(ciphertexts []types.Ciphertext, workerNum int) ([][]byte, error) {

	if !a.startRes.Done() {
		return nil, xerrors.Errorf("you must first initialize DKG. " +
			"Did you call setup() first?")
	}

	players := mino.NewAddresses(a.startRes.GetParticipants()...)

	ctx, cancel := context.WithTimeout(context.Background(), decryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameDecrypt)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, xerrors.Errorf("failed to create stream: %v", err)
	}

	players = mino.NewAddresses(a.startRes.GetParticipants()...)
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

	var respondArr []types.VerifiableDecryptReply
	// active address keeps the addresses of the nodes that actively participate in the decryption process
	var activeAddrs []mino.Address
	// receive decrypt reply from the nodes
	for i := 0; i < len(addrs); i++ {
		from, message, err := receiver.Recv(ctx)
		dela.Logger.Debug().Msgf("received the %d th share from %v\n", i, from)

		if err != nil {
			return [][]byte{}, xerrors.Errorf("stream stopped unexpectedly: %v", err)
		} else {

			shareAndProof, ok := message.(types.VerifiableDecryptReply)
			if !ok {
				return [][]byte{}, xerrors.Errorf("got unexpected reply, expected "+
					"%T but got: %T", shareAndProof, message)
			}
			respondArr = append(respondArr, shareAndProof)
			activeAddrs = append(activeAddrs, addrs[i])
		}
	}
	// the final decrypted message
	decryptedMessage := make([][]byte, batchsize)

	var wgBatchReply sync.WaitGroup
	jobChan := make(chan int, batchsize)

	for i := 0; i < batchsize; i++ {
		jobChan <- i
	}

	close(jobChan)

	if batchsize < workerNum {
		workerNum = batchsize
	}
	for i := 0; i < workerNum; i++ {
		wgBatchReply.Add(1)

		go func(jobChan <-chan int) {
			defer wgBatchReply.Done()

			for j := range jobChan {
				pubShares := make([]*share.PubShare, len(activeAddrs))

				for k := 0; k < len(activeAddrs); k++ {
					resp := respondArr[k].GetShareAndProof()[j]

					err = checkDecryptionProof(resp, ciphertexts[j].K)
					if err != nil {
						dela.Logger.Error().Msgf("failed to verify the decryption proof : %v", err)
					} else {
						pubShares[k] = &share.PubShare{
							I: int(resp.I),
							V: resp.V,
						}
					}
				}
				res, err := share.RecoverCommit(suite, pubShares, len(addrs), len(addrs))

				if err != nil {
					dela.Logger.Error().Msgf("failed to recover the message : %v", err)
				}
				decryptedMessage[j], err = res.Data()
				if err != nil {
					dela.Logger.Error().Msgf("failed to get enbeded data : %v", err)
				}
			}
		}(jobChan)
	}

	wgBatchReply.Wait()

	return decryptedMessage, nil
}

// Reshare implements dkg.Actor. It recreates the DKG with an updated list of
// participants.
func (a *Actor) Reshare(co crypto.CollectiveAuthority, thresholdNew int) error {
	if !a.startRes.Done() {
		return xerrors.Errorf("you must first initialize DKG. " +
			"Did you call setup() first?")
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
	addrsAll := merge(a.startRes.GetParticipants(), addrsNew)
	players := mino.NewAddresses(addrsAll...)

	ctx, cancel := context.WithTimeout(context.Background(), resharingTimeout)
	defer cancel()

	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameResharing)

	dela.Logger.Info().Msgf("resharing with the following participants: %v", addrsAll)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return xerrors.Errorf("failed to create stream: %v", err)
	}

	thresholdOld := a.startRes.GetThreshold()
	pubkeysOld := a.startRes.GetPublicKeys()

	// We don't need to send the old threshold or old public keys to the old or
	// common nodes
	reshare := types.NewStartResharing(thresholdNew, 0, addrsNew, nil, pubkeysNew, nil)

	dela.Logger.Info().Msgf("resharing to old participants: %v",
		a.startRes.GetParticipants())

	// Send the resharing request to the old and common nodes
	err = <-sender.Send(reshare, a.startRes.GetParticipants()...)
	if err != nil {
		return xerrors.Errorf("failed to send resharing request: %v", err)
	}

	// First find the set of new nodes that are not common between the old and
	// new committee
	newParticipants := substract(addrsNew, a.startRes.GetParticipants())

	// Then create a resharing request message for them. We should send the old
	// threshold and old public keys to them
	reshare = types.NewStartResharing(thresholdNew, thresholdOld, addrsNew,
		a.startRes.GetParticipants(), pubkeysNew, pubkeysOld)

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
			return xerrors.Errorf("stream stopped unexpectedly: %v", err)
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

// thif function verifies the decryption proof
// a discription can be found in https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3: key reconstruction
func checkDecryptionProof(sp types.ShareAndProof, K kyber.Point) error {

	tmp1 := suite.Point().Mul(sp.Fi, K)
	tmp2 := suite.Point().Mul(sp.Ei, sp.Ui)
	UHat := suite.Point().Sub(tmp1, tmp2)

	tmp1 = suite.Point().Mul(sp.Fi, nil)
	tmp2 = suite.Point().Mul(sp.Ei, sp.Hi)
	HHat := suite.Point().Sub(tmp1, tmp2)

	hash := sha256.New()
	sp.Ui.MarshalTo(hash)
	UHat.MarshalTo(hash)
	HHat.MarshalTo(hash)
	tmp := suite.Scalar().SetBytes(hash.Sum(nil))

	if !tmp.Equal(sp.Ei) {
		return xerrors.Errorf("hash is not valid")
	}
	return nil
}

// Gets the list of the old committee members and new committee members and
// returns the new committee members that are not common
func substract(addrsSlice1 []mino.Address, addrsSlice2 []mino.Address) []mino.Address {
	var result []mino.Address

	for _, addr1 := range addrsSlice1 {
		exist := false
		for _, addr2 := range addrsSlice2 {
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
