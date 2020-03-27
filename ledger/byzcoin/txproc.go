package byzcoin

import (
	"bytes"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/inventory/mem"
	"golang.org/x/xerrors"
)

// txProcessor provides primitives to pre-process transactions and commit their
// payload later on.
//
// - implements blockchain.PayloadProcessor
type txProcessor struct {
	txFactory ledger.TransactionFactory
	inventory inventory.Inventory
}

func newTxProcessor() *txProcessor {
	return &txProcessor{
		txFactory: newTransactionFactory(),
		inventory: mem.NewInventory(),
	}
}

// Validate implements blockchain.PayloadProcessor. It returns if the validation
// is a success. In that case, the payload has been staged in the inventory and
// is waiting for a commit order.
func (proc *txProcessor) Validate(index uint64, data proto.Message) error {
	payload, ok := data.(*BlockPayload)
	if !ok {
		return xerrors.Errorf("message type '%T' but expected '%T'", data, payload)
	}

	snap, err := proc.process(payload)
	if err != nil {
		return xerrors.Errorf("couldn't stage the transactions: %v", err)
	}

	if snap.GetIndex() != index {
		return xerrors.Errorf("invalid index %d != %d", snap.GetIndex(), index)
	}

	if !bytes.Equal(snap.GetFootprint(), payload.GetFootprint()) {
		return xerrors.Errorf("mismatch payload footprint '%#x' != '%#x'",
			snap.GetFootprint(), payload.GetFootprint())
	}

	fabric.Logger.Trace().Msgf("staging new inventory %x", snap.GetFootprint())

	return nil
}

func (proc *txProcessor) process(payload *BlockPayload) (inventory.Page, error) {
	return proc.inventory.Stage(func(snap inventory.WritablePage) error {
		for _, txpb := range payload.GetTxs() {
			tx, err := proc.txFactory.FromProto(txpb)
			if err != nil {
				return encoding.NewDecodingError("transaction", err)
			}

			fabric.Logger.Trace().Msgf("processing %v", tx)

			// TODO: execute the transaction with a smart contract executor.
		}

		return nil
	})
}

// Commit implements blockchain.PayloadProcessor. It tries to commit to the
// payload as it should have previously been processed. It returns nil if the
// commit is a success, otherwise an error.
func (proc *txProcessor) Commit(data proto.Message) error {
	payload, ok := data.(*BlockPayload)
	if !ok {
		return xerrors.Errorf("message type '%T' but expected '%T'", data, payload)
	}

	err := proc.inventory.Commit(payload.GetFootprint())
	if err != nil {
		return xerrors.Errorf("couldn't commit to page '%#x': %v",
			payload.GetFootprint(), err)
	}

	return nil
}
