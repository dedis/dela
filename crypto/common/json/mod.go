package json

import (
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	common.RegisterAlgorithmFormat(serde.FormatJSON, algoFormat{})
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

// AlgoFormat is the engine to encode and decode algorithm data in JSON format.
//
// - implements serde.FormatEngine
type algoFormat struct{}

// Encode implements serde.FormatEngine. It returns the JSON representation of
// an algorithm message.
func (f algoFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	algo, ok := msg.(common.Algorithm)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	m := Algorithm{
		Name: algo.GetName(),
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the algorithm message from
// the JSON data if appropriate, otherwise it returns an error.
func (f algoFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Algorithm{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize algorithm: %v", err)
	}

	alg := common.NewAlgorithm(m.Name)

	return alg, nil
}
