package byzcoin

import (
	"bytes"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/consumer/smartcontract"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/inventory/mem"
	"golang.org/x/xerrors"
)

// txProcessor provides primitives to pre-process transactions and commit their
// payload later on.
//
// - implements blockchain.PayloadProcessor
type txProcessor struct {
	encoder   encoding.ProtoMarshaler
	inventory inventory.Inventory
	consumer  consumer.Consumer
}

func newTxProcessor(c consumer.Consumer) *txProcessor {
	return &txProcessor{
		encoder:   encoding.NewProtoEncoder(),
		inventory: mem.NewInventory(),
		consumer:  c,
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
			return err
		}

		if page.GetIndex() != 0 {
			return xerrors.Errorf("index 0 expected but got %d", page.GetIndex())
		}
	case *BlockPayload:
		fabric.Logger.Trace().
			Hex("footprint", payload.GetFootprint()).
			Msgf("validating block payload")

		page, err := proc.process(payload)
		if err != nil {
			return xerrors.Errorf("couldn't stage the transactions: %v", err)
		}

		if page.GetIndex() != index {
			return xerrors.Errorf("invalid index %d != %d", page.GetIndex(), index)
		}

		if !bytes.Equal(page.GetFootprint(), payload.GetFootprint()) {
			return xerrors.Errorf("mismatch payload footprint '%#x' != '%#x'",
				page.GetFootprint(), payload.GetFootprint())
		}
	default:
		return xerrors.Errorf("invalid message type '%T'", data)
	}

	return nil
}

func (proc *txProcessor) setup(payload *GenesisPayload) (inventory.Page, error) {
	page, err := proc.inventory.Stage(func(page inventory.WritablePage) error {
		err := page.Write(authorityKey, payload.Roster)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return page, nil
}

func (proc *txProcessor) process(payload *BlockPayload) (inventory.Page, error) {
	page := proc.inventory.GetStagingPage(payload.GetFootprint())
	if page != nil {
		// Page has already been processed previously.
		return page, nil
	}

	page, err := proc.inventory.Stage(func(page inventory.WritablePage) error {
		factory := proc.consumer.GetTransactionFactory()

		for _, txpb := range payload.GetTransactions() {
			tx, err := factory.FromProto(txpb)
			if err != nil {
				return xerrors.Errorf("couldn't decode tx: %v", err)
			}

			fabric.Logger.Trace().Msgf("processing %v", tx)

			ctx := smartcontract.NewContext(tx, page)

			instance, err := proc.consumer.Consume(ctx)
			if err != nil {
				return xerrors.Errorf("couldn't consume tx: %v", err)
			}

			instancepb, err := proc.encoder.Pack(instance)
			if err != nil {
				return xerrors.Errorf("couldn't pack instance: %v", err)
			}

			err = page.Write(instance.GetKey(), instancepb)
			if err != nil {
				return xerrors.Errorf("couldn't write instances: %v", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("couldn't stage new page: %v", err)
	}

	fabric.Logger.Trace().Msgf("staging new inventory %#x", page.GetFootprint())
	return page, err
}

// Commit implements blockchain.PayloadProcessor. It tries to commit to the
// payload as it should have previously been processed. It returns nil if the
// commit is a success, otherwise an error.
func (proc *txProcessor) Commit(data proto.Message) error {
	switch payload := data.(type) {
	case *GenesisPayload:
		err := proc.inventory.Commit(payload.GetFootprint())
		if err != nil {
			return xerrors.Errorf("couldn't commit to page '%#x': %v",
				payload.GetFootprint(), err)
		}
	case *BlockPayload:
		err := proc.inventory.Commit(payload.GetFootprint())
		if err != nil {
			return xerrors.Errorf("couldn't commit to page '%#x': %v",
				payload.GetFootprint(), err)
		}
	default:
		return xerrors.Errorf("invalid message type '%T'", data)
	}

	return nil
}
