package json

import (
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, newMsgFormat())
}

type Address []byte

type PublicKey []byte

type Start struct {
	Threshold  int
	Addresses  []Address
	PublicKeys []PublicKey
}

type StartResharing struct {
	TNew       int
	TOld       int
	AddrsNew   []Address
	AddrsOld   []Address
	PubkeysNew []PublicKey
	PubkeysOld []PublicKey
}

type EncryptedDeal struct {
	DHKey     []byte
	Signature []byte
	Nonce     []byte
	Cipher    []byte
}

type Deal struct {
	Index         uint32
	Signature     []byte
	EncryptedDeal EncryptedDeal
}

type Reshare struct {
	Deal        Deal
	PublicCoeff []PublicKey
}

type DealerResponse struct {
	SessionID []byte
	Index     uint32
	Status    bool
	Signature []byte
}

type Response struct {
	Index    uint32
	Response DealerResponse
}

type StartDone struct {
	PublicKey PublicKey
}

type DecryptRequest struct {
	K []byte
	C []byte
}

type Ciphertext struct {
	K    PublicKey // r
	C    PublicKey // C
	UBar PublicKey // ubar
	E    []byte    // e
	F    []byte    // f
	GBar PublicKey // GBar
}

type VerifiableDecryptRequest struct {
	Ciphertexts []Ciphertext
}

type DecryptReply struct {
	V []byte
	I int64
}

type ShareAndProof struct {
	V  PublicKey
	I  int64
	Ui PublicKey // u_i
	Ei []byte    // e_i
	Fi []byte    // f_i
	Hi PublicKey // h_i

}

type VerifiableDecryptReply struct {
	Sp []ShareAndProof
}

type Message struct {
	Start                    *Start                    `json:",omitempty"`
	StartResharing           *StartResharing           `json:",omitempty"`
	Deal                     *Deal                     `json:",omitempty"`
	Reshare                  *Reshare                  `json:",omitempty"`
	Response                 *Response                 `json:",omitempty"`
	StartDone                *StartDone                `json:",omitempty"`
	DecryptRequest           *DecryptRequest           `json:",omitempty"`
	DecryptReply             *DecryptReply             `json:",omitempty"`
	VerifiableDecryptReply   *VerifiableDecryptReply   `json:",omitempty"`
	VerifiableDecryptRequest *VerifiableDecryptRequest `json:",omitempty"`
}

// MsgFormat is the engine to encode and decode dkg messages in JSON format.
//
// - implements serde.FormatEngine
type msgFormat struct {
	suite suites.Suite
}

func newMsgFormat() msgFormat {
	return msgFormat{
		suite: suites.MustFind("Ed25519"),
	}
}

// Encode implements serde.FormatEngine. It returns the serialized data for the
// message in JSON format.
func (f msgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m Message
	var err error

	switch in := msg.(type) {
	case types.Start:
		m, err = encodeStart(in)
	case types.StartResharing:
		m, err = encodeStartResharing(in)
	case types.Deal:
		d := Deal{
			Index:     in.GetIndex(),
			Signature: in.GetSignature(),
			EncryptedDeal: EncryptedDeal{
				DHKey:     in.GetEncryptedDeal().GetDHKey(),
				Signature: in.GetEncryptedDeal().GetSignature(),
				Nonce:     in.GetEncryptedDeal().GetNonce(),
				Cipher:    in.GetEncryptedDeal().GetCipher(),
			},
		}

		m = Message{Deal: &d}
	case types.Reshare:
		m, err = encodeReshare(in)
	case types.Response:
		r := Response{
			Index: in.GetIndex(),
			Response: DealerResponse{
				SessionID: in.GetResponse().GetSessionID(),
				Index:     in.GetResponse().GetIndex(),
				Status:    in.GetResponse().GetStatus(),
				Signature: in.GetResponse().GetSignature(),
			},
		}

		m = Message{Response: &r}
	case types.StartDone:
		m, err = encodeStartDone(in)
	case types.DecryptRequest:
		m, err = encodeDecryptRequest(in)
	case types.VerifiableDecryptRequest:
		m, err = encodeVerifiableDecryptRequest(in)
	case types.DecryptReply:
		m, err = encodeDecryptReply(in)
	case types.VerifiableDecryptReply:
		m, err = encodeVerifiableDecryptReply(in)
	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	if err != nil {
		return nil, xerrors.Errorf("failed to encode message: %v", err)
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the message from the JSON
// data if appropriate, otherwise it returns an error.
func (f msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	switch {
	case m.Start != nil:
		return f.decodeStart(ctx, m.Start)

	case m.StartResharing != nil:
		return f.decodeStartResharing(ctx, m.StartResharing)

	case m.Deal != nil:
		deal := types.NewDeal(
			m.Deal.Index,
			m.Deal.Signature,
			types.NewEncryptedDeal(
				m.Deal.EncryptedDeal.DHKey,
				m.Deal.EncryptedDeal.Signature,
				m.Deal.EncryptedDeal.Nonce,
				m.Deal.EncryptedDeal.Cipher,
			),
		)

		return deal, nil

	case m.Reshare != nil:
		return f.decodeReshare(ctx, m.Reshare)

	case m.Response != nil:
		resp := types.NewResponse(
			m.Response.Index,
			types.NewDealerResponse(
				m.Response.Response.Index,
				m.Response.Response.Status,
				m.Response.Response.SessionID,
				m.Response.Response.Signature,
			),
		)

		return resp, nil

	case m.StartDone != nil:
		return f.decodeStartDone(ctx, m.StartDone)

	case m.DecryptRequest != nil:
		return f.decodeDecryptRequest(ctx, m.DecryptRequest)

	case m.VerifiableDecryptRequest != nil:
		return f.decodeVerifiableDecryptRequest(ctx, m.VerifiableDecryptRequest)

	case m.DecryptReply != nil:
		return f.decodeDecryptReply(ctx, m.DecryptReply)

	case m.VerifiableDecryptReply != nil:
		return f.decodeVerifiableDecryptReply(ctx, m.VerifiableDecryptReply)
	}

	return nil, xerrors.New("message is empty")
}

func encodeStart(msg types.Start) (Message, error) {
	addrs := make([]Address, len(msg.GetAddresses()))
	for i, addr := range msg.GetAddresses() {
		data, err := addr.MarshalText()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		addrs[i] = data
	}

	pubkeys := make([]PublicKey, len(msg.GetPublicKeys()))
	for i, pubkey := range msg.GetPublicKeys() {
		data, err := pubkey.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		pubkeys[i] = data
	}

	start := Start{
		Threshold:  msg.GetThreshold(),
		Addresses:  addrs,
		PublicKeys: pubkeys,
	}

	return Message{Start: &start}, nil
}

func (f msgFormat) decodeStart(ctx serde.Context, start *Start) (serde.Message, error) {
	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	addrs := make([]mino.Address, len(start.Addresses))
	for i, addr := range start.Addresses {
		addrs[i] = fac.FromText(addr)
	}

	pubkeys := make([]kyber.Point, len(start.PublicKeys))
	for i, pubkey := range start.PublicKeys {
		point := f.suite.Point()
		err := point.UnmarshalBinary(pubkey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		pubkeys[i] = point
	}

	s := types.NewStart(start.Threshold, addrs, pubkeys)

	return s, nil
}

func encodeStartResharing(msg types.StartResharing) (Message, error) {
	addrsNew := make([]Address, len(msg.GetAddrsNew()))
	for i, addr := range msg.GetAddrsNew() {
		data, err := addr.MarshalText()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal new address: %v", err)
		}

		addrsNew[i] = data
	}

	addrsOld := make([]Address, len(msg.GetAddrsOld()))
	for i, addr := range msg.GetAddrsOld() {
		data, err := addr.MarshalText()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal old address: %v", err)
		}

		addrsOld[i] = data
	}

	pubkeysNew := make([]PublicKey, len(msg.GetPubkeysNew()))
	for i, pubkey := range msg.GetPubkeysNew() {
		data, err := pubkey.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal new public key: %v", err)
		}

		pubkeysNew[i] = data
	}

	pubkeysOld := make([]PublicKey, len(msg.GetPubkeysOld()))
	for i, pubkey := range msg.GetPubkeysOld() {
		data, err := pubkey.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal old public key: %v", err)
		}

		pubkeysOld[i] = data
	}

	resharingRequest := StartResharing{
		TNew:       msg.GetTNew(),
		TOld:       msg.GetTOld(),
		AddrsNew:   addrsNew,
		AddrsOld:   addrsOld,
		PubkeysNew: pubkeysNew,
		PubkeysOld: pubkeysOld,
	}

	return Message{StartResharing: &resharingRequest}, nil
}

func (f msgFormat) decodeStartResharing(ctx serde.Context,
	msg *StartResharing) (serde.Message, error) {

	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	addrsNew := make([]mino.Address, len(msg.AddrsNew))
	for i, addr := range msg.AddrsNew {
		addrsNew[i] = fac.FromText(addr)
	}

	addrsOld := make([]mino.Address, len(msg.AddrsOld))
	for i, addr := range msg.AddrsOld {
		addrsOld[i] = fac.FromText(addr)
	}

	pubkeysNew := make([]kyber.Point, len(msg.PubkeysNew))
	for i, pubkey := range msg.PubkeysNew {
		point := f.suite.Point()
		err := point.UnmarshalBinary(pubkey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal new public key: %v", err)
		}

		pubkeysNew[i] = point
	}

	pubkeysOld := make([]kyber.Point, len(msg.PubkeysOld))
	for i, pubkey := range msg.PubkeysOld {
		point := f.suite.Point()
		err := point.UnmarshalBinary(pubkey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal old public key: %v", err)
		}

		pubkeysOld[i] = point
	}

	s := types.NewStartResharing(msg.TNew, msg.TOld, addrsNew,
		addrsOld, pubkeysNew, pubkeysOld)

	return s, nil
}

func encodeReshare(msg types.Reshare) (Message, error) {
	d := Deal{
		Index:     msg.GetDeal().GetIndex(),
		Signature: msg.GetDeal().GetSignature(),
		EncryptedDeal: EncryptedDeal{
			DHKey:     msg.GetDeal().GetEncryptedDeal().GetDHKey(),
			Signature: msg.GetDeal().GetEncryptedDeal().GetSignature(),
			Nonce:     msg.GetDeal().GetEncryptedDeal().GetNonce(),
			Cipher:    msg.GetDeal().GetEncryptedDeal().GetCipher(),
		},
	}

	publicCoeff := make([]PublicKey, len(msg.GetPublicCoeffs()))
	for i, coeff := range msg.GetPublicCoeffs() {
		data, err := coeff.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal public coefficient: %v", err)
		}
		publicCoeff[i] = data
	}

	dr := Reshare{
		Deal:        d,
		PublicCoeff: publicCoeff,
	}

	return Message{Reshare: &dr}, nil
}

func (f msgFormat) decodeReshare(ctx serde.Context,
	msg *Reshare) (serde.Message, error) {

	deal := types.NewDeal(
		msg.Deal.Index,
		msg.Deal.Signature,
		types.NewEncryptedDeal(
			msg.Deal.EncryptedDeal.DHKey,
			msg.Deal.EncryptedDeal.Signature,
			msg.Deal.EncryptedDeal.Nonce,
			msg.Deal.EncryptedDeal.Cipher,
		),
	)

	publicCoeff := make([]kyber.Point, len(msg.PublicCoeff))

	for i, coeff := range msg.PublicCoeff {
		point := f.suite.Point()
		err := point.UnmarshalBinary(coeff)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal public coeff key: %v", err)
		}

		publicCoeff[i] = point
	}

	return types.NewReshare(deal, publicCoeff), nil
}

func encodeStartDone(msg types.StartDone) (Message, error) {
	pubkey, err := msg.GetPublicKey().MarshalBinary()
	if err != nil {
		return Message{}, xerrors.Errorf("couldn't marshal public key: %v", err)
	}

	ack := StartDone{
		PublicKey: pubkey,
	}

	return Message{StartDone: &ack}, nil
}

func (f msgFormat) decodeStartDone(ctx serde.Context, msg *StartDone) (serde.Message, error) {
	point := f.suite.Point()
	err := point.UnmarshalBinary(msg.PublicKey)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
	}

	ack := types.NewStartDone(point)

	return ack, nil
}

func encodeDecryptRequest(msg types.DecryptRequest) (Message, error) {
	k, err := msg.GetK().MarshalBinary()
	if err != nil {
		return Message{}, xerrors.Errorf("couldn't marshal K: %v", err)
	}

	c, err := msg.GetC().MarshalBinary()
	if err != nil {
		return Message{}, xerrors.Errorf("couldn't marshal C: %v", err)
	}

	req := DecryptRequest{
		K: k,
		C: c,
	}

	return Message{DecryptRequest: &req}, nil
}

func (f msgFormat) decodeDecryptRequest(ctx serde.Context, msg *DecryptRequest) (serde.Message, error) {
	k := f.suite.Point()
	err := k.UnmarshalBinary(msg.K)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal K: %v", err)
	}

	c := f.suite.Point()
	err = c.UnmarshalBinary(msg.C)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal C: %v", err)
	}

	req := types.NewDecryptRequest(k, c)

	return req, nil
}

func encodeVerifiableDecryptRequest(msg types.VerifiableDecryptRequest) (Message, error) {
	ciphertexts := msg.GetCiphertexts()
	var encodedCiphertexts []Ciphertext

	for _, cp := range ciphertexts {
		K, err := cp.K.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal K: %v", err)
		}

		C, err := cp.C.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal C: %v", err)
		}

		UBar, err := cp.UBar.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal UBar: %v", err)
		}

		E, err := cp.E.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal E: %v", err)
		}

		F, err := cp.F.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal F: %v", err)
		}

		GBar, err := cp.GBar.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal GBar: %v", err)
		}

		encodedCp := Ciphertext{
			C:    C,
			K:    K,
			UBar: UBar,
			E:    E,
			F:    F,
			GBar: GBar,
		}

		encodedCiphertexts = append(encodedCiphertexts, encodedCp)
	}

	req := VerifiableDecryptRequest{
		Ciphertexts: encodedCiphertexts,
	}

	return Message{VerifiableDecryptRequest: &req}, nil
}

func (f msgFormat) decodeVerifiableDecryptRequest(ctx serde.Context,
	msg *VerifiableDecryptRequest) (serde.Message, error) {

	ciphertexts := msg.Ciphertexts
	decodedCiphertexts := []types.Ciphertext{}

	for _, cp := range ciphertexts {
		K := f.suite.Point()
		err := K.UnmarshalBinary(cp.K)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal K: %v", err)
		}

		C := f.suite.Point()
		err = C.UnmarshalBinary(cp.C)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal C: %v", err)
		}

		UBar := f.suite.Point()
		err = UBar.UnmarshalBinary(cp.UBar)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal Ubar: %v", err)
		}

		E := f.suite.Scalar()
		err = E.UnmarshalBinary(cp.E)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal E: %v", err)
		}

		F := f.suite.Scalar()
		err = F.UnmarshalBinary(cp.F)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal F: %v", err)
		}

		GBar := f.suite.Point()
		err = GBar.UnmarshalBinary(cp.GBar)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal GBar: %v", err)
		}

		decodedCp := types.Ciphertext{
			K:    K,
			C:    C,
			UBar: UBar,
			E:    E,
			F:    F,
			GBar: GBar,
		}
		decodedCiphertexts = append(decodedCiphertexts, decodedCp)
	}

	resp := types.NewVerifiableDecryptRequest(decodedCiphertexts)

	return resp, nil
}

func encodeDecryptReply(msg types.DecryptReply) (Message, error) {
	v, err := msg.GetV().MarshalBinary()
	if err != nil {
		return Message{}, xerrors.Errorf("couldn't marshal V: %v", err)
	}

	resp := DecryptReply{
		V: v,
		I: msg.GetI(),
	}

	return Message{DecryptReply: &resp}, nil
}

func (f msgFormat) decodeDecryptReply(ctx serde.Context, msg *DecryptReply) (serde.Message, error) {
	v := f.suite.Point()
	err := v.UnmarshalBinary(msg.V)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal V: %v", err)
	}

	resp := types.NewDecryptReply(msg.I, v)

	return resp, nil
}

func encodeVerifiableDecryptReply(msg types.VerifiableDecryptReply) (Message, error) {
	sps := msg.GetShareAndProof()
	var encodedSps []ShareAndProof

	for _, sp := range sps {
		V, err := sp.V.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal V: %v", err)
		}

		Ui, err := sp.Ui.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal U_i: %v", err)
		}

		Ei, err := sp.Ei.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal E_i: %v", err)
		}

		Fi, err := sp.Fi.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal F_i: %v", err)
		}

		Hi, err := sp.Hi.MarshalBinary()
		if err != nil {
			return Message{}, xerrors.Errorf("couldn't marshal H_i: %v", err)
		}

		encodedSp := ShareAndProof{
			V:  V,
			I:  sp.I,
			Ui: Ui,
			Ei: Ei,
			Fi: Fi,
			Hi: Hi,
		}
		encodedSps = append(encodedSps, encodedSp)
	}

	req := VerifiableDecryptReply{
		Sp: encodedSps,
	}

	return Message{VerifiableDecryptReply: &req}, nil
}

func (f msgFormat) decodeVerifiableDecryptReply(ctx serde.Context,
	msg *VerifiableDecryptReply) (serde.Message, error) {

	sps := msg.Sp
	decodedSps := []types.ShareAndProof{}

	for _, sp := range sps {
		V := f.suite.Point()
		err := V.UnmarshalBinary(sp.V)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal V: %v", err)
		}

		Ei := f.suite.Scalar()
		err = Ei.UnmarshalBinary(sp.Ei)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal E_i: %v", err)
		}

		Ui := f.suite.Point()
		err = Ui.UnmarshalBinary(sp.Ui)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal U_i: %v", err)
		}

		Fi := f.suite.Scalar()
		err = Fi.UnmarshalBinary(sp.Fi)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal F_i: %v", err)
		}

		Hi := f.suite.Point()
		err = Hi.UnmarshalBinary(sp.Hi)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal H_i: %v", err)
		}

		decodedSp := types.ShareAndProof{
			V:  V,
			I:  sp.I,
			Ui: Ui,
			Ei: Ei,
			Fi: Fi,
			Hi: Hi,
		}

		decodedSps = append(decodedSps, decodedSp)
	}

	resp := types.NewVerifiableDecryptReply(decodedSps)

	return resp, nil
}
