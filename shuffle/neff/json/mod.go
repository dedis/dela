package json

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/shuffle/neff/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, NewMsgFormat())
}

type Address []byte

type PublicKey []byte

type ElementOfCiphertextPair []byte

type StartShuffle struct {
	Threshold  int
	ElectionId string
	Addresses  []Address
}

type EndShuffle struct {

}

type ShuffleMessage struct {
	Addresses  []Address
	Kbar []ElementOfCiphertextPair
	Cbar []ElementOfCiphertextPair
	PubKey PublicKey
	KbarPrevious []ElementOfCiphertextPair
	CbarPrevious []ElementOfCiphertextPair
	SuiteName string
	Prf []byte
}

type Message struct {
	StartShuffle   *StartShuffle   `json:",omitempty"`
	EndShuffle     *EndShuffle     `json:",omitempty"`
	ShuffleMessage *ShuffleMessage `json:",omitempty"`
}

// MsgFormat is the engine to encode and decode SHUFFLE messages in JSON format.
//
// - implements serde.FormatEngine
type MsgFormat struct {
	suite suites.Suite
}

func NewMsgFormat() MsgFormat {
	return MsgFormat{
		suite: suites.MustFind("Ed25519"),
	}
}

// Encode implements serde.FormatEngine. It returns the serialized data for the
// message in JSON format.
func (f MsgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m Message

	switch in := msg.(type) {

	case types.StartShuffle:

		addrs := make([]Address, len(in.GetAddresses()))
		for i, addr := range in.GetAddresses() {
			data, err := addr.MarshalText()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal address: %v", err)
			}

			addrs[i] = data
		}

		startShuffle := StartShuffle{
			Threshold:  in.GetThreshold(),
			ElectionId: in.GetElectionId(),
			Addresses:  addrs,
		}
		m = Message{StartShuffle: &startShuffle}

	case types.EndShuffle:
		endShuffle := EndShuffle{}
		m = Message{EndShuffle: &endShuffle}

	case types.ShuffleMessage:

		addrs := make([]Address, len(in.GetAddresses()))
		for i, addr := range in.GetAddresses() {
			data, err := addr.MarshalText()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal address: %v", err)
			}

			addrs[i] = data
		}

		pubKey := in.GetPublicKey()
		pubKeyMarshalled, err := pubKey.MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		kBar := make([]ElementOfCiphertextPair, len(in.GetkBar()))
		for i, k := range in.GetkBar() {
			data, err := k.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal kyber.Point: %v", err)
			}

			kBar[i] = data
		}

		cBar := make([]ElementOfCiphertextPair, len(in.GetcBar()))
		for i, c := range in.GetcBar() {
			data, err := c.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal kyber.Point: %v", err)
			}

			cBar[i] = data
		}

		kBarPrevious := make([]ElementOfCiphertextPair, len(in.GetkBarPrevious()))
		for i, kPrevious := range in.GetkBarPrevious() {
			data, err := kPrevious.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal kyber.Point: %v", err)
			}

			kBarPrevious[i] = data
		}

		cBarPrevious := make([]ElementOfCiphertextPair, len(in.GetcBarPrevious()))
		for i, cPrevious := range in.GetcBarPrevious() {
			data, err := cPrevious.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal kyber.Point: %v", err)
			}

			cBarPrevious[i] = data
		}

		shuffleMessage := ShuffleMessage{
			Addresses:  addrs,
			Kbar: kBar,
			Cbar: cBar,
			PubKey: pubKeyMarshalled,
			KbarPrevious: kBarPrevious,
			CbarPrevious: cBarPrevious,
			SuiteName: in.GetSuiteName(),
			Prf: in.GetProof(),
		}

		m = Message{ShuffleMessage: &shuffleMessage}

	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the message from the JSON
// data if appropriate, otherwise it returns an error.
func (f MsgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.StartShuffle != nil {
		return f.decodeStartShuffle(ctx, m.StartShuffle)
	}

	if m.EndShuffle != nil {
		return f.decodeEndShuffle(ctx, m.EndShuffle)
	}

	if m.ShuffleMessage != nil {
		return f.decodeShuffleMessage(ctx, m.ShuffleMessage)
	}

	return nil, xerrors.New("message is empty")
}

func (f MsgFormat) decodeStartShuffle(ctx serde.Context, startShuffle *StartShuffle) (serde.Message, error) {
	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	addrs := make([]mino.Address, len(startShuffle.Addresses))
	for i, addr := range startShuffle.Addresses {
		addrs[i] = fac.FromText(addr)
	}


	s := types.NewStartShuffle(startShuffle.Threshold, startShuffle.ElectionId, addrs)

	return s, nil
}

func (f MsgFormat) decodeEndShuffle(ctx serde.Context, endShuffle *EndShuffle) (serde.Message, error) {
	e := types.NewEndShuffle()

	return e, nil
}

func (f MsgFormat) decodeShuffleMessage(ctx serde.Context, shuffle *ShuffleMessage) (serde.Message, error) {
	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	addrs := make([]mino.Address, len(shuffle.Addresses))
	for i, addr := range shuffle.Addresses {
		addrs[i] = fac.FromText(addr)
	}

	pubKey := f.suite.Point()
	err := pubKey.UnmarshalBinary(shuffle.PubKey)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
	}

	kBar := make([]kyber.Point, len(shuffle.Kbar))
	for i, k := range shuffle.Kbar {
		point := f.suite.Point()
		err := point.UnmarshalBinary(k)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal kyber.Point: %v", err)
		}

		kBar[i] = point
	}

	cBar := make([]kyber.Point, len(shuffle.Cbar))
	for i, c := range shuffle.Cbar {
		point := f.suite.Point()
		err := point.UnmarshalBinary(c)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal kyber.Point: %v", err)
		}

		cBar[i] = point
	}

	kBarPrevious := make([]kyber.Point, len(shuffle.KbarPrevious))
	for i, kPrevious := range shuffle.KbarPrevious {
		point := f.suite.Point()
		err := point.UnmarshalBinary(kPrevious)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal kyber.Point: %v", err)
		}

		kBarPrevious[i] = point
	}

	cBarPrevious := make([]kyber.Point, len(shuffle.CbarPrevious))
	for i, cPrevious := range shuffle.CbarPrevious {
		point := f.suite.Point()
		err := point.UnmarshalBinary(cPrevious)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal kyber.Point: %v", err)
		}

		cBarPrevious[i] = point
	}

	s := types.NewShuffleMessage(addrs, shuffle.SuiteName, pubKey, kBar, cBar, kBarPrevious, cBarPrevious, shuffle.Prf)

	return s, nil
}