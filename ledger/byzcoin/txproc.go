package byzcoin

import (
	"bytes"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// txProcessor provides primitives to pre-process transactions and commit their
// payload later on.
//
// - implements blockchain.PayloadProcessor
type txProcessor struct {
	MessageFactory

	inventory inventory.Inventory
}

func newTxProcessor(f MessageFactory, i inventory.Inventory) *txProcessor {
	return &txProcessor{
		MessageFactory: f,
		inventory:      i,
	}
}

// Validate implements blockchain.PayloadProcessor. It returns if the validation
// is a success. In that case, the payload has been staged in the inventory and
// is waiting for a commit order.
func (proc *txProcessor) InvokeValidate(data serde.Message) (blockchain.Payload, error) {
	blueprint, ok := data.(Blueprint)
	if !ok {
		return nil, xerrors.Errorf("invalid message type '%T'", data)
	}

	payload := BlockPayload{
		transactions: blueprint.transactions,
		root:         nil,
	}

	page, err := proc.process(payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't stage the transactions: %v", err)
	}

	payload.root = page.GetFingerprint()

	return payload, nil
}

// Commit implements blockchain.PayloadProcessor. It tries to commit to the
// payload as it should have previously been processed. It returns nil if the
// commit is a success, otherwise an error.
func (proc *txProcessor) InvokeCommit(p blockchain.Payload) error {
	var root []byte

	switch payload := p.(type) {
	case GenesisPayload:
		page, err := proc.setup(payload)
		if err != nil {
			return xerrors.Errorf("couldn't stage genesis: %v", err)
		}

		if page.GetIndex() != 0 {
			return xerrors.Errorf("index 0 expected but got %d", page.GetIndex())
		}

		root = payload.root
	case BlockPayload:
		page, err := proc.process(payload)
		if err != nil {
			return xerrors.Errorf("couldn't process: %v", err)
		}

		if !bytes.Equal(page.GetFingerprint(), payload.root) {
			return xerrors.Errorf("mismatch root '%#x' != '%#x'",
				page.GetFingerprint(), payload.root)
		}

		root = payload.root
	default:
		return xerrors.Errorf("invalid message type '%T'", p)
	}

	err := proc.inventory.Commit(root)
	if err != nil {
		return xerrors.Errorf("couldn't commit to page '%#x': %v", root, err)
	}

	return nil
}

func (proc *txProcessor) setup(payload GenesisPayload) (inventory.Page, error) {
	page, err := proc.inventory.Stage(func(page inventory.WritablePage) error {
		err := page.Write(rosterValueKey, payload.roster)
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

func (proc *txProcessor) process(payload BlockPayload) (inventory.Page, error) {
	page := proc.inventory.GetStagingPage(payload.root)
	if page != nil {
		// Page has already been processed previously.
		return page, nil
	}

	page, err := proc.inventory.Stage(func(page inventory.WritablePage) error {
		for _, tx := range payload.transactions {
			dela.Logger.Trace().Msgf("processing %v", tx)

			err := tx.Consume(page)
			if err != nil {
				return xerrors.Errorf("couldn't consume tx: %v", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("couldn't stage new page: %v", err)
	}

	dela.Logger.Trace().Msgf("staging new inventory %#x", page.GetFingerprint())
	return page, err
}
