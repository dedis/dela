package contract

import (
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

// Instance is the message stored for a contract.
type Instance struct {
	Key           []byte
	Value         serdeng.Message
	ContractID    string
	Deleted       bool
	AccessControl []byte
}

func (i *Instance) Serialize(serdeng.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}
