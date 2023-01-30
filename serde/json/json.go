// Package json implements the context engine for a the JSON format.
//
// Documentation Last Review: 07.10.2020
package json

import (
	"encoding/json"

	// Static registration of the JSON formats. By having them here, it ensures
	// that an import of the JSON context engine will import the definitions.
	_ "go.dedis.ch/dela/core/access/darc/json"
	_ "go.dedis.ch/dela/core/ordering/cosipbft/authority/json"
	_ "go.dedis.ch/dela/core/ordering/cosipbft/blocksync/json"
	_ "go.dedis.ch/dela/core/ordering/cosipbft/json"
	_ "go.dedis.ch/dela/core/txn/signed/json"
	_ "go.dedis.ch/dela/core/validation/simple/json"
	_ "go.dedis.ch/dela/cosi/json"
	_ "go.dedis.ch/dela/cosi/threshold/json"
	_ "go.dedis.ch/dela/crypto/bls/json"
	_ "go.dedis.ch/dela/crypto/ed25519/json"
	_ "go.dedis.ch/dela/dkg/pedersen/json"
	_ "go.dedis.ch/dela/mino/router/tree/json"
	"go.dedis.ch/dela/serde"
)

// JSONEngine is a context engine to marshal and unmarshal in JSON format.
//
// - implements serde.ContextEngine
type jsonEngine struct{}

// NewContext returns a JSON context.
func NewContext() serde.Context {
	return serde.NewContext(jsonEngine{})
}

// GetFormat implements serde.FormatEngine. It returns the JSON format name.
func (ctx jsonEngine) GetFormat() serde.Format {
	return serde.FormatJSON
}

// Marshal implements serde.FormatEngine. It returns the bytes of the message
// marshaled in JSON format.
func (ctx jsonEngine) Marshal(m interface{}) ([]byte, error) {
	return json.Marshal(m)
}

// Unmarshal implements serde.FormatEngine. It populates the message using the
// JSON format definition.
func (ctx jsonEngine) Unmarshal(data []byte, m interface{}) error {
	return json.Unmarshal(data, m)
}
