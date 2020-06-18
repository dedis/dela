package json

import (
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func init() {
	common.Register(serdeng.CodecJSON, format{})
}

// Algorithm is a common JSON message to identify which algorithm is used in a
// message.
type Algorithm struct {
	Name string
}

// PublicKey is the common JSON message for a public key. It contains the
// algorithm and the data to deserialize.
type PublicKey struct {
	Algorithm
	Data []byte
}

// Signature is the common JSON message for a signature. It contains the
// algorithm and the data to deserialize.
type Signature struct {
	Algorithm
	Data []byte
}

type format struct{}

func (f format) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}

func (f format) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := Algorithm{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize algorithm: %v", err)
	}

	alg := common.NewAlgorithm(m.Name)

	return alg, nil
}
