// This file contains the implementation of the views sent by the nodes during a
// view change.
//
// Documentation Last Review: 13.10.2020
//

package pbft

import (
	"encoding/binary"

	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// View is the view change request sent to other participants.
type View struct {
	from      mino.Address
	id        types.Digest
	leader    uint16
	signature crypto.Signature
}

// ViewParam contains the parameters to create a view.
type ViewParam struct {
	From   mino.Address
	ID     types.Digest
	Leader uint16
}

// NewView creates a new view.
func NewView(param ViewParam, sig crypto.Signature) View {
	return View{
		from:      param.From,
		id:        param.ID,
		leader:    param.Leader,
		signature: sig,
	}
}

// NewViewAndSign creates a new view and uses the signer to make the signature
// that will be verified by other participants.
func NewViewAndSign(param ViewParam, signer crypto.Signer) (View, error) {
	view := View{
		from:   param.From,
		id:     param.ID,
		leader: param.Leader,
	}

	sig, err := signer.Sign(view.bytes())
	if err != nil {
		return view, xerrors.Errorf("signer: %v", err)
	}

	view.signature = sig

	return view, nil
}

// GetFrom returns the address the view is coming from.
func (v View) GetFrom() mino.Address {
	return v.from
}

// GetID returns the block digest the view is targeting.
func (v View) GetID() types.Digest {
	return v.id
}

// GetLeader returns the index of the leader proposed by the view.
func (v View) GetLeader() uint16 {
	return v.leader
}

// GetSignature returns the signature of the view.
func (v View) GetSignature() crypto.Signature {
	return v.signature
}

// Verify takes the public key to verify the signature of the view.
func (v View) Verify(pubkey crypto.PublicKey) error {
	err := pubkey.Verify(v.bytes(), v.signature)
	if err != nil {
		return xerrors.Errorf("verify: %v", err)
	}

	return nil
}

func (v View) bytes() []byte {
	buffer := make([]byte, 2)
	binary.LittleEndian.PutUint16(buffer, v.leader)

	return append(buffer, v.id.Bytes()...)
}
