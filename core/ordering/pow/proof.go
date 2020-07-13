package pow

import "go.dedis.ch/dela/core/store"

// Proof is a proof that the chain of blocks has or has not the key in the
// store. If the key exists, the proof also contains the value.
//
// - implements ordering.Proof
type Proof struct {
	blocks []Block
	key    []byte
	share  store.Share
}

// GetKey implements ordering.Proof.
func (p Proof) GetKey() []byte {
	return p.key
}

// GetValue implements ordering.Proof.
func (p Proof) GetValue() []byte {
	return p.share.GetValue()
}
