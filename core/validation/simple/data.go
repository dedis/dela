package simple

import (
	"io"

	"go.dedis.ch/dela/core/tap"
	"golang.org/x/xerrors"
)

// TransactionResult is the result of a transaction processing. It contains the
// transaction and its state of success.
type TransactionResult struct {
	tx       tap.Transaction
	accepted bool
}

// Data is the validated data of a standard validation.
type Data struct {
	root []byte
	txs  []TransactionResult
}

// Fingerprint writes a deterministic binary representation of the validated
// data.
func (d Data) Fingerprint(w io.Writer) error {
	_, err := w.Write(d.root)
	if err != nil {
		return xerrors.Errorf("couldn't write root: %v", err)
	}

	for _, tx := range d.txs {
		err = tx.tx.Fingerprint(w)
		if err != nil {
			return xerrors.Errorf("couldn't fingerprint tx: %v", err)
		}

		bit := []byte{0}
		if tx.accepted {
			bit[0] = 1
		}

		_, err = w.Write(bit)
		if err != nil {
			return xerrors.Errorf("couldn't write accepted: %v", err)
		}
	}

	return nil
}
