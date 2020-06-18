package pedersen

import (
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
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
	rpc     mino.RPC
}

// NewPedersen returns a new DKG Pedersen factory
func NewPedersen(privKey kyber.Scalar, m mino.Mino) (*Pedersen, error) {
	m, err := m.MakeNamespace("dkg")
	if err != nil {
		return nil, xerrors.Errorf("failed to create namespace: %v", err)
	}

	factory := NewMessageFactory(suite, m.GetAddressFactory())

	h := NewHandler(privKey, m.GetAddress(), factory)

	// TODO: should be done only in the Listen func.
	rpc, err := m.MakeRPC("pedersen", h, factory)
	if err != nil {
		return nil, xerrors.Errorf("failed to create RPC: %v", err)
	}

	return &Pedersen{
		privKey: privKey,
		mino:    m,
		rpc:     rpc,
	}, nil
}

// Listen implements dkg.DKG. It allows one to start a new DKG protocol.
func (s *Pedersen) Listen(players mino.Players, pubKeys []kyber.Point, t int) (dkg.Actor, error) {
	if players.Len() != len(pubKeys) {
		return nil, xerrors.Errorf("there should be as many players as "+
			"pubKey: %d := %d", players.Len, len(pubKeys))
	}

	newPlayers := players.Take(mino.RangeFilter(0, players.Len()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver, err := s.rpc.Stream(ctx, newPlayers)
	if err != nil {
		return nil, err
	}

	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}

	message := Start{
		t:         t,
		addresses: addrs,
		pubkeys:   pubKeys,
	}

	errs := sender.Send(message, addrs...)
	err, more := <-errs
	if more {
		return nil, xerrors.Errorf("failed to send start: %v", err)
	}

	dkgPubKeys := make([]kyber.Point, len(addrs))

	for i := 0; i < len(addrs); i++ {

		addr, msg, err := receiver.Recv(context.Background())
		if err != nil {
			return nil, xerrors.Errorf("got an error from '%s' while "+
				"receiving: %v", addr, err)
		}

		doneMsg, ok := msg.(StartDone)
		if !ok {
			return nil, xerrors.Errorf("expected to receive a Done message, but "+
				"go the following: %T", msg)
		}

		dkgPubKeys[i] = doneMsg.pubkey

		// this is a simple check that every node sends back the same DKG pub
		// key.
		// TODO: handle the situation where a pub key is not the same
		if i != 0 && !dkgPubKeys[i-1].Equal(doneMsg.pubkey) {
			return nil, xerrors.Errorf("the public keys does not match: %v", dkgPubKeys)
		}
	}

	// TODO: that only works for the one that runs listen.
	actor := &Actor{
		rpc:     s.rpc,
		PubKey:  dkgPubKeys[0],
		players: players,
	}

	return actor, nil
}

// Actor allows one to perform DKG operations like encrypt/decrypt a message
//
// - implements dkg.Actor
type Actor struct {
	rpc     mino.RPC
	PubKey  kyber.Point
	players mino.Players
}

// Encrypt implements dkg.DKG. It uses the DKG public key to encrypt a message.
func (p *Actor) Encrypt(message []byte) (K, C kyber.Point, remainder []byte,
	err error) {

	// Embed the message (or as much of it as will fit) into a curve point.
	M := suite.Point().Embed(message, random.New())
	max := suite.Point().EmbedLen()
	if max > len(message) {
		max = len(message)
	}
	remainder = message[max:]
	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := suite.Scalar().Pick(random.New()) // ephemeral private key
	K = suite.Point().Mul(k, nil)          // ephemeral DH public key
	S := suite.Point().Mul(k, p.PubKey)    // ephemeral DH shared secret
	C = S.Add(S, M)                        // message blinded with secret

	return K, C, remainder, nil
}

// Decrypt implements dkg.DKG. It gets the private shares of the nodes and
// decrypt the  message.
// TODO: perform a re-encryption instead of gathering the private shares, which
// should never happen.
func (p *Actor) Decrypt(K, C kyber.Point) ([]byte, error) {
	if p.PubKey == nil {
		return nil, xerrors.New("pubkey is nil, did you call Start() first?")
	}

	players := p.players.Take(mino.RangeFilter(0, p.players.Len()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver, err := p.rpc.Stream(ctx, players)
	if err != nil {
		return nil, nil
	}

	players = p.players.Take(mino.RangeFilter(0, p.players.Len()))
	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}

	message := DecryptRequest{
		K: K,
		C: C,
	}

	sender.Send(message, addrs...)

	pubShares := make([]*share.PubShare, len(addrs))

	for i := 0; i < len(addrs); i++ {
		from, message, err := receiver.Recv(ctx)
		if err != nil {
			return []byte{}, xerrors.Errorf("failed to receive from '%s': %v",
				from, err)
		}

		decryptReply, ok := message.(DecryptReply)
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

// Reshare implements dkg.DKG. It recreates the DKG with an updated list of
// participants.
// TODO: to do
func (p *Actor) Reshare() error {
	return nil
}
