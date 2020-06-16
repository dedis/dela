package json

type Address []byte

type PublicKey []byte

type Start struct {
	Threshold  int
	Addresses  []Address
	PublicKeys []PublicKey
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

type DecryptReply struct {
	V []byte
	I int64
}

type Message struct {
	Start          *Start
	Deal           *Deal
	Response       *Response
	StartDone      *StartDone
	DecryptRequest *DecryptRequest
	DecryptReply   *DecryptReply
}
