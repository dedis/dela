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

type txProcessor struct {
	txFactory ledger.TransactionFactory
	inventory inventory.Inventory
}

func newValidator() *txProcessor {
	return &txProcessor{
		txFactory: newTransactionFactory(),
		inventory: mem.NewInventory(),
	}
}

func (proc *txProcessor) Validate(index uint64, data proto.Message) error {
	payload, ok := data.(*BlockPayload)
	if !ok {
		return xerrors.Errorf("message type '%T' but expected '%T'", data, payload)
	}

	snap, err := proc.process(payload)
	if err != nil {
		return xerrors.Errorf("couldn't stage the transactions: %v", err)
	}

	if snap.GetVersion() != index {
		return xerrors.Errorf("invalid index %d != %d", snap.GetVersion(), index)
	}

	if !bytes.Equal(snap.GetRoot(), payload.GetRoot()) {
		return xerrors.Errorf("mismatch payload root %x != %x",
			snap.GetRoot(), payload.GetRoot())
	}

	fabric.Logger.Info().Msgf("staging new inventory %x", snap.GetRoot())

	return nil
}

func (proc *txProcessor) process(payload *BlockPayload) (inventory.Snapshot, error) {
	return proc.inventory.Stage(func(snap inventory.WritableSnapshot) error {
		for _, txpb := range payload.GetTxs() {
			tx, err := proc.txFactory.FromProto(txpb)
			if err != nil {
				return encoding.NewDecodingError("transaction", err)
			}

			fabric.Logger.Info().Msgf("processing %v", tx)
		}

		return nil
	})
}

func (proc *txProcessor) Commit(data proto.Message) error {
	payload, ok := data.(*BlockPayload)
	if !ok {
		return xerrors.Errorf("message type '%T' but expected '%T'", data, payload)
	}

	err := proc.inventory.Commit(payload.GetRoot())
	if err != nil {
		return xerrors.Errorf("couldn't commit to snapshot %x: %v", payload.GetRoot(), err)
	}

	return nil
}
