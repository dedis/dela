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

type ResharingRequest struct {
	T_new       int
	T_old       int
	Addrs_new   []Address
	Addrs_old   []Address
	Pubkeys_new []PublicKey
	Pubkeys_old []PublicKey
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

type Deal_resharing struct {
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
	UBar PublicKey //ubar
	E    []byte    //e
	F    []byte    //f
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
	ResharingRequest         *ResharingRequest         `json:",omitempty"`
	Deal                     *Deal                     `json:",omitempty"`
	Deal_resharing           *Deal_resharing           `json:",omitempty"`
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

	switch in := msg.(type) {
	case types.Start:
		addrs := make([]Address, len(in.GetAddresses()))
		for i, addr := range in.GetAddresses() {
			data, err := addr.MarshalText()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal address: %v", err)
			}

			addrs[i] = data
		}

		pubkeys := make([]PublicKey, len(in.GetPublicKeys()))
		for i, pubkey := range in.GetPublicKeys() {
			data, err := pubkey.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
			}

			pubkeys[i] = data
		}

		start := Start{
			Threshold:  in.GetThreshold(),
			Addresses:  addrs,
			PublicKeys: pubkeys,
		}

		m = Message{Start: &start}

	case types.ResharingRequest:

		resharingRequest, err := f.encodeResharingRequest(in)
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		m = Message{ResharingRequest: &resharingRequest}
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

	case types.Deal_resharing:
		d := Deal{
			Index:     in.GetDeal().GetIndex(),
			Signature: in.GetDeal().GetSignature(),
			EncryptedDeal: EncryptedDeal{
				DHKey:     in.GetDeal().GetEncryptedDeal().GetDHKey(),
				Signature: in.GetDeal().GetEncryptedDeal().GetSignature(),
				Nonce:     in.GetDeal().GetEncryptedDeal().GetNonce(),
				Cipher:    in.GetDeal().GetEncryptedDeal().GetCipher(),
			},
		}

		publicCoeff := make([]PublicKey, len(in.GetPublicCoeffs()))
		for i, coeff := range in.GetPublicCoeffs() {
			data, err := coeff.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal public coeffcient %v", err)
			}
			publicCoeff[i] = data
		}

		dr := Deal_resharing{
			Deal:        d,
			PublicCoeff: publicCoeff,
		}
		m = Message{Deal_resharing: &dr}

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
		pubkey, err := in.GetPublicKey().MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		ack := StartDone{
			PublicKey: pubkey,
		}

		m = Message{StartDone: &ack}
	case types.DecryptRequest:
		k, err := in.GetK().MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal K: %v", err)
		}

		c, err := in.GetC().MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal C: %v", err)
		}

		req := DecryptRequest{
			K: k,
			C: c,
		}

		m = Message{DecryptRequest: &req}

	case types.VerifiableDecryptRequest:

		chiphertexts := in.GetCiphertexts()
		var encodedCiphertexts []Ciphertext
		for _, cp := range chiphertexts {
			K, err := cp.K.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal K: %v", err)
			}

			C, err := cp.C.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal C: %v", err)
			}

			UBar, err := cp.UBar.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal UBar: %v", err)
			}

			E, err := cp.E.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal E: %v", err)
			}

			F, err := cp.F.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal F: %v", err)
			}

			GBar, err := cp.GBar.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal GBar: %v", err)
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

		m = Message{VerifiableDecryptRequest: &req}

	case types.DecryptReply:
		v, err := in.GetV().MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal V: %v", err)
		}

		resp := DecryptReply{
			V: v,
			I: in.GetI(),
		}

		m = Message{DecryptReply: &resp}

	case types.VerifiableDecryptReply:
		sps := in.GetShareAndProof()
		var encodedSps []ShareAndProof
		for _, sp := range sps {
			V, err := sp.V.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal V: %v", err)
			}

			Ui, err := sp.Ui.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal U_i: %v", err)
			}

			Ei, err := sp.Ei.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal E_i: %v", err)
			}

			Fi, err := sp.Fi.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal F_i: %v", err)
			}

			Hi, err := sp.Hi.MarshalBinary()
			if err != nil {
				return nil, xerrors.Errorf("couldn't marshal H_i: %v", err)
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

		m = Message{VerifiableDecryptReply: &req}

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
func (f msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.Start != nil {
		return f.decodeStart(ctx, m.Start)
	}

	if m.ResharingRequest != nil {
		return f.decodeResharingRequest(ctx, m.ResharingRequest)
	}

	if m.Deal != nil {
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
	}

	if m.Deal_resharing != nil {
		deal := types.NewDeal(
			m.Deal_resharing.Deal.Index,
			m.Deal_resharing.Deal.Signature,
			types.NewEncryptedDeal(
				m.Deal_resharing.Deal.EncryptedDeal.DHKey,
				m.Deal_resharing.Deal.EncryptedDeal.Signature,
				m.Deal_resharing.Deal.EncryptedDeal.Nonce,
				m.Deal_resharing.Deal.EncryptedDeal.Cipher,
			),
		)

		publicCoeff := make([]kyber.Point, len(m.Deal_resharing.PublicCoeff))
		for i, coeff := range m.Deal_resharing.PublicCoeff {
			point := f.suite.Point()
			err := point.UnmarshalBinary(coeff)
			if err != nil {
				return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
			}

			publicCoeff[i] = point
		}

		deal_resharing := types.NewDeal_resharing(deal, publicCoeff)

		return deal_resharing, nil
	}

	if m.Response != nil {
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
	}

	if m.StartDone != nil {
		point := f.suite.Point()
		err := point.UnmarshalBinary(m.StartDone.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		ack := types.NewStartDone(point)

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

		req := types.NewDecryptRequest(k, c)

		return req, nil
	}

	if m.VerifiableDecryptRequest != nil {

		ciphertexts := m.VerifiableDecryptRequest.Ciphertexts
		var decodedCiphertexts []types.Ciphertext

		for _, cp := range ciphertexts {
			K := f.suite.Point()
			err = K.UnmarshalBinary(cp.K)
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

	if m.DecryptReply != nil {
		v := f.suite.Point()
		err = v.UnmarshalBinary(m.DecryptReply.V)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal V: %v", err)
		}

		resp := types.NewDecryptReply(m.DecryptReply.I, v)

		return resp, nil
	}

	if m.VerifiableDecryptReply != nil {

		sps := m.VerifiableDecryptReply.Sp
		var decodedSps []types.ShareAndProof

		for _, sp := range sps {
			V := f.suite.Point()
			err = V.UnmarshalBinary(sp.V)
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

	return nil, xerrors.New("message is empty")
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

func (f msgFormat) encodeResharingRequest(in types.ResharingRequest) (ResharingRequest, error) {

	addrs_new := make([]Address, len(in.GetAddrs_new()))
	for i, addr := range in.GetAddrs_new() {
		data, err := addr.MarshalText()
		if err != nil {
			return ResharingRequest{}, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		addrs_new[i] = data
	}

	addrs_old := make([]Address, len(in.GetAddrs_old()))
	for i, addr := range in.GetAddrs_old() {
		data, err := addr.MarshalText()
		if err != nil {
			return ResharingRequest{}, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		addrs_old[i] = data
	}

	pubkeys_new := make([]PublicKey, len(in.GetPubkeys_new()))
	for i, pubkey := range in.GetPubkeys_new() {
		data, err := pubkey.MarshalBinary()
		if err != nil {
			return ResharingRequest{}, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		pubkeys_new[i] = data
	}

	pubkeys_old := make([]PublicKey, len(in.GetPubkeys_old()))
	for i, pubkey := range in.GetPubkeys_old() {
		data, err := pubkey.MarshalBinary()
		if err != nil {
			return ResharingRequest{}, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		pubkeys_old[i] = data
	}

	resharingRequest := ResharingRequest{
		T_new:       in.GetT_new(),
		T_old:       in.GetT_old(),
		Addrs_new:   addrs_new,
		Addrs_old:   addrs_old,
		Pubkeys_new: pubkeys_new,
		Pubkeys_old: pubkeys_old,
	}

	return resharingRequest, nil

}

func (f msgFormat) decodeResharingRequest(ctx serde.Context, resharingRequest *ResharingRequest) (serde.Message, error) {

	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	addrs_new := make([]mino.Address, len(resharingRequest.Addrs_new))
	for i, addr := range resharingRequest.Addrs_new {
		addrs_new[i] = fac.FromText(addr)
	}

	addrs_old := make([]mino.Address, len(resharingRequest.Addrs_old))
	for i, addr := range resharingRequest.Addrs_old {
		addrs_old[i] = fac.FromText(addr)
	}

	pubkeys_new := make([]kyber.Point, len(resharingRequest.Pubkeys_new))
	for i, pubkey := range resharingRequest.Pubkeys_new {
		point := f.suite.Point()
		err := point.UnmarshalBinary(pubkey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		pubkeys_new[i] = point
	}

	pubkeys_old := make([]kyber.Point, len(resharingRequest.Pubkeys_old))
	for i, pubkey := range resharingRequest.Pubkeys_old {
		point := f.suite.Point()
		err := point.UnmarshalBinary(pubkey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		pubkeys_old[i] = point
	}

	s := types.NewResharingRequest(resharingRequest.T_new, resharingRequest.T_old, addrs_new,
		addrs_old, pubkeys_new, pubkeys_old)

	return s, nil

}
