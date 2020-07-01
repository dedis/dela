package json

import (
	"encoding/json"

	// Static registration of the JSON formats.
	_ "go.dedis.ch/dela/blockchain/skipchain/json"
	_ "go.dedis.ch/dela/consensus/cosipbft/json"
	_ "go.dedis.ch/dela/consensus/qsc/json"
	_ "go.dedis.ch/dela/consensus/viewchange/roster/json"
	_ "go.dedis.ch/dela/cosi/json"
	_ "go.dedis.ch/dela/crypto/bls/json"
	_ "go.dedis.ch/dela/dkg/pedersen/json"
	_ "go.dedis.ch/dela/ledger/arc/darc/json"
	_ "go.dedis.ch/dela/ledger/byzcoin/json"
	_ "go.dedis.ch/dela/ledger/byzcoin/memship/json"
	_ "go.dedis.ch/dela/ledger/transactions/basic/json"
	_ "go.dedis.ch/dela/mino/minogrpc/routing/json"
	"go.dedis.ch/dela/serde"
)

type jsonEngine struct{}

// NewContext returns a JSON context.
func NewContext() serde.Context {
	return serde.NewContext(jsonEngine{})
}

func (ctx jsonEngine) GetName() serde.Codec {
	return serde.CodecJSON
}

func (ctx jsonEngine) Marshal(m interface{}) ([]byte, error) {
	return json.Marshal(m)
}

func (ctx jsonEngine) Unmarshal(data []byte, m interface{}) error {
	return json.Unmarshal(data, m)
}
