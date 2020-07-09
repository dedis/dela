package anon

import (
	"io"

	"go.dedis.ch/dela/ledger/arc"
)

type Transaction struct{}

func NewTransaction() Transaction {
	return Transaction{}
}

func (t Transaction) GetID() []byte {
	return []byte{}
}

func (t Transaction) GetIdentity() arc.Identity {
	return nil
}

func (t Transaction) GetArg(key string) []byte {
	return []byte{}
}

func (t Transaction) Fingerprint(io.Writer) error {
	return nil
}
