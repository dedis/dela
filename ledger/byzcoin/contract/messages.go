package contract

import (
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Instance is the message stored for a contract.
type Instance struct {
	Key           []byte
	Value         serde.Message
	ContractID    string
	Deleted       bool
	AccessControl []byte
}

func (i *Instance) Serialize(serde.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}
