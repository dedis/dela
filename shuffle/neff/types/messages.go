package types

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

var msgFormats = registry.NewSimpleRegistry()

// RegisterMessageFormat register the engine for the provided format.
func RegisterMessageFormat(c serde.Format, f serde.FormatEngine) {
	msgFormats.Register(c, f)
}

// StartShuffle is the message the initiator of the DKG protocol should send to all the
// nodes.
//
// - implements serde.Message
type StartShuffle struct {
	threshold  int
	electionId string
	addresses []mino.Address
}

// NewStart creates a new start message.
func NewStartShuffle(threshold int, electionId string, addresses []mino.Address) StartShuffle {
	return StartShuffle{
		threshold:   threshold,
		electionId : electionId,
		addresses : addresses,
	}
}

// GetThreshold returns the threshold.
func (s StartShuffle) GetThreshold() int {
	return s.threshold
}

// GetElectionId returns the electionId.
func (s StartShuffle) GetElectionId() string {
	return s.electionId
}

// GetAddresses returns the list of addresses.
func (s StartShuffle) GetAddresses() []mino.Address {
	return append([]mino.Address{}, s.addresses...)
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the start message.
func (s StartShuffle) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode StartShuffle message: %v", err)
	}

	return data, nil
}

type EndShuffle struct {
}

func NewEndShuffle() EndShuffle {
	return EndShuffle{
	}
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the start message.
func (e EndShuffle) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode EndShuffle message: %v", err)
	}

	return data, nil
}

// ShuffleMessage is a message that contains every information needed to run the shuffle protocol.
//
// - implements serde.Message
type ShuffleMessage struct {

	// The full list of addresses that will participate in the shuffle protocol.
	addresses []mino.Address

	// The name of the curve.
	suiteName	string

	// The kyber.Point public key used to generate the ElGamal pairs.
	publicKey kyber.Point

	// The corresponding list of kyber.Point ElGamal pairs to shuffle.
	kBar []kyber.Point
	cBar []kyber.Point

	// The previously shuffled list of kyber.Point ElGamal pairs.
	kBarPrevious []kyber.Point
	cBarPrevious []kyber.Point

	// The proof of the previous shuffle.
	prf []byte

}

// NewShuffleMessage creates a new ShuffleMessage.
func NewShuffleMessage(addresses []mino.Address, suiteName	string, publicKey kyber.Point, kBar []kyber.Point, cBar []kyber.Point,
	kBarPrevious []kyber.Point, cBarPrevious []kyber.Point, prf []byte) ShuffleMessage {
	return ShuffleMessage{
		addresses:	addresses,
		suiteName:	suiteName,
		publicKey:	publicKey,
		kBar:	kBar,
		cBar:   cBar,
		kBarPrevious: 	kBarPrevious,
		cBarPrevious:	cBarPrevious,
		prf:	prf,
	}
}

// GetAddresses returns the list of addresses.
func (s ShuffleMessage) GetAddresses() []mino.Address {
	return append([]mino.Address{}, s.addresses...)
}

// GetSuiteName returns the name of the suite.
func (s ShuffleMessage) GetSuiteName() string {
	return s.suiteName
}

// GetPublicKey returns the public key.
func (s ShuffleMessage) GetPublicKey() kyber.Point {
	return s.publicKey
}

// GetkBar and GetcBar return the list of ElGamal pairs to shuffle.
func (s ShuffleMessage) GetkBar() []kyber.Point {
	return append([]kyber.Point{}, s.kBar...)
}

// GetkBar and GetcBar return the list of ElGamal pairs to shuffle.
func (s ShuffleMessage) GetcBar() []kyber.Point {
	return append([]kyber.Point{}, s.cBar...)
}

// GetkBarPrevious and GetcBarPrevious return the list of the previously shuffled ElGamal pairs.
func (s ShuffleMessage) GetkBarPrevious() []kyber.Point {
	return append([]kyber.Point{}, s.kBarPrevious...)
}

// GetkBarPrevious and GetcBarPrevious return the list of the previously shuffled ElGamal pairs.
func (s ShuffleMessage) GetcBarPrevious() []kyber.Point {
	return append([]kyber.Point{}, s.cBarPrevious...)
}

// GetProof returns the proof of the previous shuffle.
func (s ShuffleMessage) GetProof() []byte {
	return append([]byte{}, s.prf...)
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the ShuffleMessage.
func (s ShuffleMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())
	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode message : %v", err)
	}

	return data, nil
}

// AddrKey is the key for the address factory.
type AddrKey struct{}

// MessageFactory is a message factory for the different SHUFFLE messages.
//
// - implements serde.Factory
type MessageFactory struct {
	addrFactory mino.AddressFactory
}

// NewMessageFactory returns a message factory for the shuffle protocol.
func NewMessageFactory(f mino.AddressFactory) MessageFactory {
	return MessageFactory{
		addrFactory: f,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode message: %v", err)
	}

	return msg, nil
}
