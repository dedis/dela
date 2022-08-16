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

// Start is the message the initiator of the DKG protocol should send to all the
// nodes.
//
// - implements serde.Message
type Start struct {
	// threshold
	thres int
	// the full list of addresses that will participate in the DKG
	addresses []mino.Address
	// the corresponding kyber.Point pub keys of the addresses
	pubkeys []kyber.Point
}

// NewStart creates a new start message.
func NewStart(thres int, addrs []mino.Address, pubkeys []kyber.Point) Start {
	return Start{
		thres:     thres,
		addresses: addrs,
		pubkeys:   pubkeys,
	}
}

// GetThreshold returns the threshold.
func (s Start) GetThreshold() int {
	return s.thres
}

// GetAddresses returns the list of addresses.
func (s Start) GetAddresses() []mino.Address {
	return append([]mino.Address{}, s.addresses...)
}

// GetPublicKeys returns the list of public keys.
func (s Start) GetPublicKeys() []kyber.Point {
	return append([]kyber.Point{}, s.pubkeys...)
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the start message.
func (s Start) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode message: %v", err)
	}

	return data, nil
}

// ResharingRequest is the message the initiator of the resjaring protocol should send to all the
// old nodes.
//
// - implements serde.Message
type ResharingRequest struct {
	// new threshold
	T_new int
	// old threshold
	T_old int
	// the full list of addresses that will participate in the new DKG
	addrs_new []mino.Address
	// the full list of addresses of old dkg members
	addrs_old []mino.Address
	// the corresponding kyber.Point pub keys of the new addresses
	pubkeys_new []kyber.Point
	// the corresponding kyber.Point pub keys of the old addresses
	pubkeys_old []kyber.Point
}

// NewResharingRequest creates a new start message.
func NewResharingRequest(T_new int, T_old int, addrs_new []mino.Address, addrs_old []mino.Address,
	pubkeys_new []kyber.Point, pubkeys_old []kyber.Point) ResharingRequest {
	return ResharingRequest{
		T_new:       T_new,
		T_old:       T_old,
		addrs_new:   addrs_new,
		addrs_old:   addrs_old,
		pubkeys_new: pubkeys_new,
		pubkeys_old: pubkeys_old,
	}
}

// GetT_new returns the new threshold.
func (r ResharingRequest) GetT_new() int {
	return r.T_new
}

// GetT_old returns the old threshold.
func (r ResharingRequest) GetT_old() int {
	return r.T_old
}

//  GetAddrs_new returns the list of new addresses.
func (r ResharingRequest) GetAddrs_new() []mino.Address {
	return append([]mino.Address{}, r.addrs_new...)
}

//  GetAddrs_old returns the list of old addresses.
func (r ResharingRequest) GetAddrs_old() []mino.Address {
	return append([]mino.Address{}, r.addrs_old...)
}

//  GetPubkeys_new returns the list of new public keys.
func (r ResharingRequest) GetPubkeys_new() []kyber.Point {
	return append([]kyber.Point{}, r.pubkeys_new...)
}

//  GetPubkeys_old returns the list of old public keys.
func (r ResharingRequest) GetPubkeys_old() []kyber.Point {
	return append([]kyber.Point{}, r.pubkeys_old...)
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the resharingRequest message.
func (r ResharingRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, r)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode message: %v", err)
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

// NewEncryptedDeal creates a new encrypted deal message.
func NewEncryptedDeal(dhkey, sig, nonce, cipher []byte) EncryptedDeal {
	return EncryptedDeal{
		dhkey:     dhkey,
		signature: sig,
		nonce:     nonce,
		cipher:    cipher,
	}
}

// GetDHKey returns the Diffie-Helmann key in bytes.
func (d EncryptedDeal) GetDHKey() []byte {
	return append([]byte{}, d.dhkey...)
}

// GetSignature returns the signatures in bytes.
func (d EncryptedDeal) GetSignature() []byte {
	return append([]byte{}, d.signature...)
}

// GetNonce returns the nonce in bytes.
func (d EncryptedDeal) GetNonce() []byte {
	return append([]byte{}, d.nonce...)
}

// GetCipher returns the cipher in bytes.
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

// NewDeal creates a new deal.
func NewDeal(index uint32, sig []byte, e EncryptedDeal) Deal {
	return Deal{
		index:         index,
		signature:     sig,
		encryptedDeal: e,
	}
}

// GetIndex returns the index.
func (d Deal) GetIndex() uint32 {
	return d.index
}

// GetSignature returns the signature in bytes.
func (d Deal) GetSignature() []byte {
	return append([]byte{}, d.signature...)
}

// GetEncryptedDeal returns the encrypted deal.
func (d Deal) GetEncryptedDeal() EncryptedDeal {
	return d.encryptedDeal
}

// Serialize implements serde.Message.
func (d Deal) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, d)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode deal: %v", err)
	}

	return data, nil
}

// deal messages for resharing process
// - implements serde.Message
type Deal_resharing struct {
	deal        Deal
	PublicCoeff []kyber.Point
}

// NewDeal creates a new deal.
func NewDeal_resharing(deal Deal, publicCoeff []kyber.Point) Deal_resharing {
	return Deal_resharing{
		deal:        deal,
		PublicCoeff: publicCoeff,
	}
}

// GetDeal returns the deal.
func (d Deal_resharing) GetDeal() Deal {
	return d.deal
}

// GetDPublicCoeff returns the public coeff.
func (d Deal_resharing) GetPublicCoeffs() []kyber.Point {
	return d.PublicCoeff
}

// Serialize implements serde.Message.
func (d Deal_resharing) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, d)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode deal: %v", err)
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

// NewDealerResponse creates a new dealer response.
func NewDealerResponse(index uint32, status bool, sessionID, sig []byte) DealerResponse {
	return DealerResponse{
		sessionID: sessionID,
		index:     index,
		status:    status,
		signature: sig,
	}
}

// GetSessionID returns the session ID in bytes.
func (dresp DealerResponse) GetSessionID() []byte {
	return append([]byte{}, dresp.sessionID...)
}

// GetIndex returns the index.
func (dresp DealerResponse) GetIndex() uint32 {
	return dresp.index
}

// GetStatus returns the status.
func (dresp DealerResponse) GetStatus() bool {
	return dresp.status
}

// GetSignature returns the signature in bytes.
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

// NewResponse creates a new response.
func NewResponse(index uint32, r DealerResponse) Response {
	return Response{
		index:    index,
		response: r,
	}
}

// GetIndex returns the index.
func (r Response) GetIndex() uint32 {
	return r.index
}

// GetResponse returns the dealer response.
func (r Response) GetResponse() DealerResponse {
	return r.response
}

// Serialize implements serde.Message.
func (r Response) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, r)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode response: %v", err)
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

// NewStartDone creates a new start done message.
func NewStartDone(pubkey kyber.Point) StartDone {
	return StartDone{
		pubkey: pubkey,
	}
}

// GetPublicKey returns the public key of the LTS.
func (s StartDone) GetPublicKey() kyber.Point {
	return s.pubkey
}

// Serialize implements serde.Message.
func (s StartDone) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode ack: %v", err)
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

// NewDecryptRequest creates a new decryption request.
func NewDecryptRequest(k, c kyber.Point) DecryptRequest {
	return DecryptRequest{
		K: k,
		C: c,
	}
}

// GetK returns K.
func (req DecryptRequest) GetK() kyber.Point {
	return req.K
}

// GetC returns C.
func (req DecryptRequest) GetC() kyber.Point {
	return req.C
}

// Serialize implements serde.Message.
func (req DecryptRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode decrypt request: %v", err)
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

// NewDecryptReply returns a new decryption reply.
func NewDecryptReply(i int64, v kyber.Point) DecryptReply {
	return DecryptReply{
		I: i,
		V: v,
	}
}

// GetV returns V.
func (resp DecryptReply) GetV() kyber.Point {
	return resp.V
}

// GetI returns I.
func (resp DecryptReply) GetI() int64 {
	return resp.I
}

// Serialize implements serde.Message.
func (resp DecryptReply) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode decrypt reply: %v", err)
	}

	return data, nil
}

// AddrKey is the key for the address factory.
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
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode message: %v", err)
	}

	return msg, nil
}
