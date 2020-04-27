package pedersen

import (
	"fmt"

	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/random"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./messages.proto

// Pedersen ...
type Pedersen struct {
	handler *Handler
	rpc     mino.RPC
	PubKey  kyber.Point
	suite   suites.Suite
	players mino.Players
}

// NewPedersen ...
func NewPedersen(pubKeys []kyber.Point, privKey kyber.Scalar,
	m mino.Mino, addrs []mino.Address, suite suites.Suite) (*Pedersen, error) {

	h := NewHandler(pubKeys, privKey, m.GetAddressFactory(), m.GetAddress(),
		suite)

	m, err := m.MakeNamespace("dkg")
	if err != nil {
		return nil, xerrors.Errorf("failed to create namespace: %v", err)
	}

	rpc, err := m.MakeRPC("pedersen", h)
	if err != nil {
		return nil, xerrors.Errorf("failed to create RPC: %v", err)
	}

	return &Pedersen{
		handler: h,
		rpc:     rpc,
		suite:   suite,
	}, nil
}

// Start ...
func (p *Pedersen) Start(players mino.Players, t uint32) error {

	newPlayers := players.Take(mino.RangeFilter(0, players.Len()))
	sender, receiver := p.rpc.Stream(context.Background(), newPlayers)

	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}

	message := &Start{
		T:         t,
		Addresses: make([][]byte, len(addrs)),
	}

	for i, addr := range addrs {
		addrBuf, err := addr.MarshalText()
		if err != nil {
			return xerrors.Errorf("failed to marsahl address '%s': %v", addr, err)
		}
		message.Addresses[i] = addrBuf
	}

	errs := sender.Send(message, addrs...)
	err, more := <-errs
	if more {
		return xerrors.Errorf("failed to send start: %v", err)
	}

	pubKeys := make([]kyber.Point, len(addrs))

	for i := 0; i < len(addrs); i++ {

		addr, msg, err := receiver.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("got an error from '%s' while receiving: %v",
				addr, err)
		}

		doneMsg, ok := msg.(*StartDone)
		if !ok {
			return xerrors.Errorf("expected to receive a Done message, but "+
				"go the following: %v", msg)
		}

		pubKey := suite.Point()
		err = pubKey.UnmarshalBinary(doneMsg.PubKey)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal pubkey: %v", err)
		}

		pubKeys[i] = pubKey

		if i != 0 && !pubKeys[i-1].Equal(pubKey) {
			return xerrors.Errorf("the public keys does not match: %v", pubKeys)
		}
	}

	p.players = players
	p.PubKey = pubKeys[0]

	fmt.Println("here is the public DKG key: ", p.PubKey)

	return nil
}

// Encrypt ...
func (p *Pedersen) Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error) {
	if p.PubKey == nil {
		return nil, nil, []byte{}, xerrors.Errorf(
			"pubkey is nil, did you call Start() first?")
	}

	// Embed the message (or as much of it as will fit) into a curve point.
	M := p.suite.Point().Embed(message, random.New())
	max := p.suite.Point().EmbedLen()
	if max > len(message) {
		max = len(message)
	}
	remainder = message[max:]
	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := p.suite.Scalar().Pick(random.New()) // ephemeral private key
	K = p.suite.Point().Mul(k, nil)          // ephemeral DH public key
	S := p.suite.Point().Mul(k, p.PubKey)    // ephemeral DH shared secret
	C = S.Add(S, M)                          // message blinded with secret

	return K, C, remainder, nil
}

// Decrypt ...
func (p *Pedersen) Decrypt(K, C kyber.Point) ([]byte, error) {
	if p.PubKey == nil {
		return []byte{}, xerrors.Errorf(
			"pubkey is nil, did you call Start() first?")
	}

	KBuf, err := K.MarshalBinary()
	if err != nil {
		return []byte{}, xerrors.Errorf("failed to marshal K: %v", err)
	}

	CBuf, err := C.MarshalBinary()
	if err != nil {
		return []byte{}, xerrors.Errorf("failed to marshal C: %v", err)
	}

	players := p.players.Take(mino.RangeFilter(0, p.players.Len()))
	sender, receiver := p.rpc.Stream(context.Background(), players)

	players = p.players.Take(mino.RangeFilter(0, p.players.Len()))
	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}

	message := &Decrypt{
		K: KBuf,
		C: CBuf,
	}

	sender.Send(message, addrs...)

	pubShares := make([]*share.PubShare, len(addrs))

	for i := 0; i < len(addrs); i++ {
		from, message, err := receiver.Recv(context.Background())
		if err != nil {
			return []byte{}, xerrors.Errorf("failed to receive from '%s': %v", from, err)
		}

		decryptReply, ok := message.(*DecryptReply)
		if !ok {
			return []byte{}, xerrors.Errorf("got unexpected reply, expected "+
				"a decrypt reply but got: %v", message)
		}

		V := p.suite.Point()
		err = V.UnmarshalBinary(decryptReply.V)
		if err != nil {
			return []byte{}, xerrors.Errorf("failed to unmarshal point: %v", err)
		}

		pubShares[i] = &share.PubShare{
			I: int(decryptReply.I),
			V: V,
		}
	}

	res, err := share.RecoverCommit(p.suite, pubShares, len(addrs), len(addrs))
	if err != nil {
		return []byte{}, xerrors.Errorf("failed to recover commit: %v", err)
	}

	decryptedMessage, err := res.Data()
	if err != nil {
		return []byte{}, xerrors.Errorf("failed to get embeded data: %v", err)
	}

	return decryptedMessage, nil
}
