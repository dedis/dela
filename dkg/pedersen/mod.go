package pedersen

import (
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/random"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

// Suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

// Pedersen allows one to initialise a new DKG protocol.
//
// - implements dkg.DKG
type Pedersen struct {
	privKey kyber.Scalar
	mino    mino.Mino
	factory serde.Factory
}

// NewPedersen returns a new DKG Pedersen factory
func NewPedersen(privKey kyber.Scalar, m mino.Mino) *Pedersen {
	factory := NewMessageFactory(suite, m.GetAddressFactory())

	return &Pedersen{
		privKey: privKey,
		mino:    m,
		factory: factory,
	}
}

// Listen implements dkg.DKG. It must be called on each node that participates
// in the DKG. Creates the RPC.
func (s *Pedersen) Listen() (dkg.Actor, error) {

	h := NewHandler(s.privKey, s.mino.GetAddress(), s.factory)

	rpc, err := s.mino.MakeRPC("dkg", h)
	if err != nil {
		return nil, xerrors.Errorf("failed to create RPC: %v", err)
	}

	a := &Actor{
		rpc:      rpc,
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
	startRes *startResult
}

// Setup implement dkg.Actor. It initializes the DKG.
func (a *Actor) Setup(players mino.Players, pubKeys []kyber.Point, threshold int) error {

	if players.Len() != len(pubKeys) {
		return xerrors.Errorf("there should be as many players as "+
			"pubKey: %d := %d", players.Len(), len(pubKeys))
	}

	if a.startRes.distrKey != nil {
		return xerrors.Errorf("startRes is not nil, only one setup call is allowed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return xerrors.Errorf("failed to stream: %v", err)
	}

	players.AddressIterator().Seek(0)
	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}

	message := Start{
		t:         threshold,
		addresses: addrs,
		pubkeys:   pubKeys,
	}

	errs := sender.Send(tmp.ProtoOf(message), addrs...)
	err, more := <-errs
	if more {
		return xerrors.Errorf("failed to send start: %v", err)
	}

	dkgPubKeys := make([]kyber.Point, len(addrs))

	for i := 0; i < len(addrs); i++ {

		addr, msg, err := receiver.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("got an error from '%s' while "+
				"receiving: %v", addr, err)
		}

		resp := tmp.FromProto(msg, a.factory)

		doneMsg, ok := resp.(StartDone)
		if !ok {
			return xerrors.Errorf("expected to receive a Done message, but "+
				"go the following: %T", msg)
		}

		dkgPubKeys[i] = doneMsg.pubkey

		// this is a simple check that every node sends back the same DKG pub
		// key.
		// TODO: handle the situation where a pub key is not the same
		if i != 0 && !dkgPubKeys[i-1].Equal(doneMsg.pubkey) {
			return xerrors.Errorf("the public keys does not match: %v", dkgPubKeys)
		}
	}

	return nil
}

// Encrypt implements dkg.Actor. It uses the DKG public key to encrypt a message.
func (a *Actor) Encrypt(message []byte) (K, C kyber.Point, remainder []byte,
	err error) {

	if a.startRes.distrKey == nil {
		return nil, nil, nil, xerrors.Errorf("startRes is nil, did you call " +
			"setup() first?")
	}

	// Embed the message (or as much of it as will fit) into a curve point.
	M := suite.Point().Embed(message, random.New())
	max := suite.Point().EmbedLen()
	if max > len(message) {
		max = len(message)
	}
	remainder = message[max:]
	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := suite.Scalar().Pick(random.New())         // ephemeral private key
	K = suite.Point().Mul(k, nil)                  // ephemeral DH public key
	S := suite.Point().Mul(k, a.startRes.distrKey) // ephemeral DH shared secret
	C = S.Add(S, M)                                // message blinded with secret

	return K, C, remainder, nil
}

// Decrypt implements dkg.Actor. It gets the private shares of the nodes and
// decrypt the  message.
// TODO: perform a re-encryption instead of gathering the private shares, which
// should never happen.
func (a *Actor) Decrypt(players mino.Players, K, C kyber.Point) ([]byte, error) {

	if a.startRes.distrKey == nil {
		return nil, xerrors.Errorf("startRes is nil, did you call " +
			"setup() first?")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, nil
	}

	players.AddressIterator().Seek(0)
	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}

	message := DecryptRequest{
		K: K,
		C: C,
	}

	sender.Send(tmp.ProtoOf(message), addrs...)

	pubShares := make([]*share.PubShare, len(addrs))

	for i := 0; i < len(addrs); i++ {
		from, message, err := receiver.Recv(ctx)
		if err != nil {
			return []byte{}, xerrors.Errorf("failed to receive from '%s': %v",
				from, err)
		}

		resp := tmp.FromProto(message, a.factory)

		decryptReply, ok := resp.(DecryptReply)
		if !ok {
			return []byte{}, xerrors.Errorf("got unexpected reply, expected "+
				"a decrypt reply but got: %T", message)
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

	return decryptedMessage, nil
}

// Reshare implements dkg.Actor. It recreates the DKG with an updated list of
// participants.
// TODO: to do
func (a *Actor) Reshare() error {
	return nil
}
