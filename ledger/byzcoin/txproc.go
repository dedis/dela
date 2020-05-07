package byzcoin

import (
	"bytes"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions"
	"golang.org/x/xerrors"
)

// txProcessor provides primitives to pre-process transactions and commit their
// payload later on.
//
// - implements blockchain.PayloadProcessor
type txProcessor struct {
	inventory inventory.Inventory
	txFactory transactions.TransactionFactory
}

func newTxProcessor(f transactions.TransactionFactory, i inventory.Inventory) *txProcessor {
	return &txProcessor{
		inventory: i,
		txFactory: f,
	}
}

// Validate implements blockchain.PayloadProcessor. It returns if the validation
// is a success. In that case, the payload has been staged in the inventory and
// is waiting for a commit order.
func (proc *txProcessor) Validate(index uint64, data proto.Message) error {
	switch payload := data.(type) {
	case *GenesisPayload:
		page, err := proc.setup(payload)
		if err != nil {
			return xerrors.Errorf("couldn't stage genesis: %v", err)
		}

		if page.GetIndex() != 0 {
			return xerrors.Errorf("index 0 expected but got %d", page.GetIndex())
		}
	case *BlockPayload:
		fabric.Logger.Trace().
			Hex("fingerprint", payload.GetFingerprint()).
			Msgf("validating block payload")

		page, err := proc.process(payload)
		if err != nil {
			return xerrors.Errorf("couldn't stage the transactions: %v", err)
		}

		if page.GetIndex() != index {
			return xerrors.Errorf("invalid index %d != %d", page.GetIndex(), index)
		}

		if !bytes.Equal(page.GetFingerprint(), payload.GetFingerprint()) {
			return xerrors.Errorf("mismatch payload fingerprint '%#x' != '%#x'",
				page.GetFingerprint(), payload.GetFingerprint())
		}
	default:
		return xerrors.Errorf("invalid message type '%T'", data)
	}

	return nil
}

func (proc *txProcessor) setup(payload *GenesisPayload) (inventory.Page, error) {
	page, err := proc.inventory.Stage(func(page inventory.WritablePage) error {
		err := page.Write(rosterValueKey, payload.Roster)
		if err != nil {
			return xerrors.Errorf("couldn't write roster: %v", err)
		}

		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("couldn't stage page: %v", err)
	}

	return page, nil
}

func (proc *txProcessor) process(payload *BlockPayload) (inventory.Page, error) {
	page := proc.inventory.GetStagingPage(payload.GetFingerprint())
	if page != nil {
		// Page has already been processed previously.
		return page, nil
	}

	page, err := proc.inventory.Stage(func(page inventory.WritablePage) error {
		for _, txpb := range payload.GetTransactions() {
			tx, err := proc.txFactory.FromProto(txpb)
			if err != nil {
				return xerrors.Errorf("couldn't decode tx: %v", err)
			}

			fabric.Logger.Trace().Msgf("processing %v", tx)

			err = tx.Consume(page)
			if err != nil {
				return xerrors.Errorf("couldn't consume tx: %v", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("couldn't stage new page: %v", err)
	}

	fabric.Logger.Trace().Msgf("staging new inventory %#x", page.GetFingerprint())
	return page, err
}

// Commit implements blockchain.PayloadProcessor. It tries to commit to the
// payload as it should have previously been processed. It returns nil if the
// commit is a success, otherwise an error.
func (proc *txProcessor) Commit(data proto.Message) error {
	var fingerprint []byte

	switch payload := data.(type) {
	case *GenesisPayload:
		fingerprint = payload.GetFingerprint()
	case *BlockPayload:
		fingerprint = payload.GetFingerprint()
	default:
		return xerrors.Errorf("invalid message type '%T'", data)
	}

	err := proc.inventory.Commit(fingerprint)
	if err != nil {
		return xerrors.Errorf("couldn't commit to page '%#x': %v", fingerprint, err)
	}

	return nil
}
