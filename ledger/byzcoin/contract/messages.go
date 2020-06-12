package contract

import "go.dedis.ch/dela/serde"

// Instance is the message stored for a contract.
type Instance struct {
	serde.UnimplementedMessage

	Key           []byte
	Value         serde.Message
	ContractID    string
	Deleted       bool
	AccessControl []byte
}
