package types

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
	"go.dedis.ch/kyber/v3"
)

var msgFormats = registry.NewSimpleRegistry()

func RegisterMessageFormat(c serdeng.Codec, f serdeng.Format) {
	msgFormats.Register(c, f)
}

// Start is the message the initiator of the DKG protocol should send to all the
// nodes.
//
// - implements serde.Message
type Start struct {
	// threshold
	t int
	// the full list of addresses that will participate in the DKG
	addresses []mino.Address
	// the corresponding kyber.Point pub keys of the addresses
	pubkeys []kyber.Point
}

func NewStart(thres int, addrs []mino.Address, pubkeys []kyber.Point) Start {
	return Start{
		t:         thres,
		addresses: addrs,
		pubkeys:   pubkeys,
	}
}

func (s Start) GetThreshold() int {
	return s.t
}

func (s Start) GetAddresses() []mino.Address {
	return append([]mino.Address{}, s.addresses...)
}

func (s Start) GetPublicKeys() []kyber.Point {
	return append([]kyber.Point{}, s.pubkeys...)
}

// Serialize implements serde.Message.
func (s Start) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// EncryptedDeal contains the different parameters and data of an encrypted
// deal.
type EncryptedDeal struct {
	dhkey     []byte
	signature []byte
	nonce     []byte
	cipher    []byte
}

func NewEncryptedDeal(dhkey, sig, nonce, cipher []byte) EncryptedDeal {
	return EncryptedDeal{
		dhkey:     dhkey,
		signature: sig,
		nonce:     nonce,
		cipher:    cipher,
	}
}

func (d EncryptedDeal) GetDHKey() []byte {
	return append([]byte{}, d.dhkey...)
}

func (d EncryptedDeal) GetSignature() []byte {
	return append([]byte{}, d.signature...)
}

func (d EncryptedDeal) GetNonce() []byte {
	return append([]byte{}, d.nonce...)
}

func (d EncryptedDeal) GetCipher() []byte {
	return append([]byte{}, d.cipher...)
}

// Deal matches the attributes defined in kyber dkg.Deal.
//
// - implements serde.Message
type Deal struct {
	index     uint32
	signature []byte

	encryptedDeal EncryptedDeal
}

func NewDeal(index uint32, sig []byte, e EncryptedDeal) Deal {
	return Deal{
		index:         index,
		signature:     sig,
		encryptedDeal: e,
	}
}

func (d Deal) GetIndex() uint32 {
	return d.index
}

func (d Deal) GetSignature() []byte {
	return append([]byte{}, d.signature...)
}

func (d Deal) GetEncryptedDeal() EncryptedDeal {
	return d.encryptedDeal
}

// Serialize implements serde.Message.
func (d Deal) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, d)
	if err != nil {
		return nil, err
	}

	return data, nil
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

func NewDealerResponse(index uint32, status bool, sessionID, sig []byte) DealerResponse {
	return DealerResponse{
		sessionID: sessionID,
		index:     index,
		status:    status,
		signature: sig,
	}
}

func (dresp DealerResponse) GetSessionID() []byte {
	return append([]byte{}, dresp.sessionID...)
}

func (dresp DealerResponse) GetIndex() uint32 {
	return dresp.index
}

func (dresp DealerResponse) GetStatus() bool {
	return dresp.status
}

func (dresp DealerResponse) GetSignature() []byte {
	return append([]byte{}, dresp.signature...)
}

// Response matches the attributes defined in kyber pedersen.Response.
//
// - implements serde.Message
type Response struct {
	// Index of the Dealer this response is for.
	index    uint32
	response DealerResponse
}

func NewResponse(index uint32, r DealerResponse) Response {
	return Response{
		index:    index,
		response: r,
	}
}

func (r Response) GetIndex() uint32 {
	return r.index
}

func (r Response) GetResponse() DealerResponse {
	return r.response
}

// Serialize implements serde.Message.
func (r Response) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, r)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// StartDone should be sent by all the nodes to the initiator of the DKG when
// the DKG setup is done.
//
// - implements serde.Message
type StartDone struct {
	pubkey kyber.Point
}

func NewStartDone(pubkey kyber.Point) StartDone {
	return StartDone{
		pubkey: pubkey,
	}
}

func (s StartDone) GetPublicKey() kyber.Point {
	return s.pubkey
}

// Serialize implements serde.Message.
func (s StartDone) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// DecryptRequest is a message sent to request a decryption.
//
// - implements serde.Message
type DecryptRequest struct {
	K kyber.Point
	C kyber.Point
}

func NewDecryptRequest(k, c kyber.Point) DecryptRequest {
	return DecryptRequest{
		K: k,
		C: c,
	}
}

func (req DecryptRequest) GetK() kyber.Point {
	return req.K
}

func (req DecryptRequest) GetC() kyber.Point {
	return req.C
}

// Serialize implements serde.Message.
func (req DecryptRequest) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// DecryptReply is the response of a decryption request.
//
// - implements serde.Message
type DecryptReply struct {
	V kyber.Point
	I int64
}

func NewDecryptReply(i int64, v kyber.Point) DecryptReply {
	return DecryptReply{
		I: i,
		V: v,
	}
}

func (resp DecryptReply) GetV() kyber.Point {
	return resp.V
}

func (resp DecryptReply) GetI() int64 {
	return resp.I
}

// Serialize implements serde.Message.
func (resp DecryptReply) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type AddrKey struct{}

// MessageFactory is a message factory for the different DKG messages.
//
// - implements serde.Factory
type MessageFactory struct {
	addrFactory mino.AddressFactory
}

// NewMessageFactory returns a message factory for the DKG.
func NewMessageFactory(f mino.AddressFactory) MessageFactory {
	return MessageFactory{
		addrFactory: f,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := msgFormats.Get(ctx.GetName())

	ctx = serdeng.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
