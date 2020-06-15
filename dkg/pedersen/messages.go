package pedersen

import (
	"go.dedis.ch/dela/dkg/pedersen/json"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

// Start is the message the initiator of the DKG protocol should send to all the
// nodes.
//
// - implements serde.Message
type Start struct {
	serde.UnimplementedMessage

	// threshold
	t int
	// the full list of addresses that will participate in the DKG
	addresses []mino.Address
	// the corresponding kyber.Point pub keys of the addresses
	pubkeys []kyber.Point
}

// VisitJSON implements serde.Message. It serializes the start message in JSON
// format.
func (s Start) VisitJSON(serde.Serializer) (interface{}, error) {
	addrs := make([]json.Address, len(s.addresses))
	for i, addr := range s.addresses {
		data, err := addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		addrs[i] = data
	}

	pubkeys := make([]json.PublicKey, len(s.pubkeys))
	for i, pubkey := range s.pubkeys {
		data, err := pubkey.MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		pubkeys[i] = data
	}

	m := json.Start{
		Threshold:  s.t,
		Addresses:  addrs,
		PublicKeys: pubkeys,
	}

	return json.Message{Start: &m}, nil
}

// EncryptedDeal contains the different parameters and data of an encrypted
// deal.
type EncryptedDeal struct {
	dhkey     []byte
	signature []byte
	nonce     []byte
	cipher    []byte
}

// Deal matches the attributes defined in kyber dkg.Deal.
//
// - implements serde.Message
type Deal struct {
	serde.UnimplementedMessage

	index     uint32
	signature []byte

	encryptedDeal EncryptedDeal
}

// VisitJSON implements serde.Message. It serializes the deal in JSON format.
func (d Deal) VisitJSON(serde.Serializer) (interface{}, error) {
	m := json.Deal{
		Index:     d.index,
		Signature: d.signature,
		EncryptedDeal: json.EncryptedDeal{
			DHKey:     d.encryptedDeal.dhkey,
			Signature: d.encryptedDeal.signature,
			Nonce:     d.encryptedDeal.nonce,
			Cipher:    d.encryptedDeal.cipher,
		},
	}

	return json.Message{Deal: &m}, nil
}

// DealerResponse is a response of a single dealer.
type DealerResponse struct {
	sessionID []byte
	// Index of the verifier issuing this Response from the new set of
	// nodes.
	index     uint32
	status    bool
	signature []byte
}

// Response matches the attributes defined in kyber pedersen.Response.
//
// - implements serde.Message
type Response struct {
	serde.UnimplementedMessage

	// Index of the Dealer this response is for.
	index    uint32
	response DealerResponse
}

// VisitJSON implements serde.Message. It serializes the response in JSON
// format.
func (r Response) VisitJSON(serde.Serializer) (interface{}, error) {
	m := json.Response{
		Index: r.index,
		Response: json.DealerResponse{
			SessionID: r.response.sessionID,
			Index:     r.response.index,
			Status:    r.response.status,
			Signature: r.response.signature,
		},
	}

	return json.Message{Response: &m}, nil
}

// StartDone should be sent by all the nodes to the initiator of the DKG when
// the DKG setup is done.
//
// - implements serde.Message
type StartDone struct {
	serde.UnimplementedMessage

	pubkey kyber.Point
}

// VisitJSON implements serde.Message. It serializes the message to announce the
// end of the start in JSON format.
func (s StartDone) VisitJSON(serde.Serializer) (interface{}, error) {
	pubkey, err := s.pubkey.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
	}

	m := json.StartDone{
		PublicKey: pubkey,
	}

	return json.Message{StartDone: &m}, nil
}

// DecryptRequest is a message sent to request a decryption.
//
// - implements serde.Message
type DecryptRequest struct {
	serde.UnimplementedMessage

	K kyber.Point
	C kyber.Point
}

// VisitJSON implements serde.Message. It serializes the decryption request in
// JSON format.
func (req DecryptRequest) VisitJSON(serde.Serializer) (interface{}, error) {
	k, err := req.K.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal K: %v", err)
	}

	c, err := req.C.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal C: %v", err)
	}

	m := json.DecryptRequest{
		K: k,
		C: c,
	}

	return json.Message{DecryptRequest: &m}, nil
}

// DecryptReply is the response of a decryption request.
//
// - implements serde.Message
type DecryptReply struct {
	serde.UnimplementedMessage

	V kyber.Point
	I int64
}

// VisitJSON implements serde.Message. It serializes the response of the
// decryption request in JSON format.
func (resp DecryptReply) VisitJSON(serde.Serializer) (interface{}, error) {
	v, err := resp.V.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal V: %v", err)
	}

	m := json.DecryptReply{
		V: v,
		I: resp.I,
	}

	return json.Message{DecryptReply: &m}, nil
}

// MessageFactory is a message factory for the different DKG messages.
//
// - implements serde.Factory
type MessageFactory struct {
	serde.UnimplementedFactory

	suite       suites.Suite
	addrFactory mino.AddressFactory
}

// NewMessageFactory returns a message factory for the DKG.
func NewMessageFactory(suite suites.Suite, f mino.AddressFactory) serde.Factory {
	return MessageFactory{
		suite:       suite,
		addrFactory: f,
	}
}

// VisitJSON implements serde.Factory. It deserializes the DKG messages in JSON
// format.
func (f MessageFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Message{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.Start != nil {
		return f.deserializeStart(m.Start)
	}

	if m.Deal != nil {
		deal := Deal{
			index:     m.Deal.Index,
			signature: m.Deal.Signature,
			encryptedDeal: EncryptedDeal{
				nonce:     m.Deal.EncryptedDeal.Nonce,
				dhkey:     m.Deal.EncryptedDeal.DHKey,
				signature: m.Deal.EncryptedDeal.Signature,
				cipher:    m.Deal.EncryptedDeal.Cipher,
			},
		}

		return deal, nil
	}

	if m.Response != nil {
		resp := Response{
			index: m.Response.Index,
			response: DealerResponse{
				sessionID: m.Response.Response.SessionID,
				index:     m.Response.Response.Index,
				signature: m.Response.Response.Signature,
				status:    m.Response.Response.Status,
			},
		}

		return resp, nil
	}

	if m.StartDone != nil {
		point := f.suite.Point()
		err := point.UnmarshalBinary(m.StartDone.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		ack := StartDone{pubkey: point}

		return ack, nil
	}

	if m.DecryptRequest != nil {
		k := f.suite.Point()
		err = k.UnmarshalBinary(m.DecryptRequest.K)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal K: %v", err)
		}

		c := f.suite.Point()
		err = c.UnmarshalBinary(m.DecryptRequest.C)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal C: %v", err)
		}

		req := DecryptRequest{
			K: k,
			C: c,
		}

		return req, nil
	}

	if m.DecryptReply != nil {
		v := f.suite.Point()
		err = v.UnmarshalBinary(m.DecryptReply.V)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal V: %v", err)
		}

		resp := DecryptReply{
			V: v,
			I: m.DecryptReply.I,
		}

		return resp, nil
	}

	return nil, xerrors.New("message is empty")
}

func (f MessageFactory) deserializeStart(start *json.Start) (Start, error) {
	addrs := make([]mino.Address, len(start.Addresses))
	for i, addr := range start.Addresses {
		addrs[i] = f.addrFactory.FromText(addr)
	}

	pubkeys := make([]kyber.Point, len(start.PublicKeys))
	for i, pubkey := range start.PublicKeys {
		point := f.suite.Point()
		err := point.UnmarshalBinary(pubkey)
		if err != nil {
			return Start{}, xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		pubkeys[i] = point
	}

	s := Start{
		t:         start.Threshold,
		addresses: addrs,
		pubkeys:   pubkeys,
	}

	return s, nil
}
