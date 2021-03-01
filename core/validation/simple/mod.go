// Package simple implements a validation service that executes a batch
// of transactions sequentially.
//
// Documentation Last Review: 08.10.2020
//
package simple

import (
	"encoding/binary"
	"fmt"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// Service is a standard validation service that will process the batch and
// update the snapshot accordingly.
//
// - implements validation.Service
type Service struct {
	execution execution.Service
	fac       validation.ResultFactory
	hashFac   crypto.HashFactory
}

// NewService creates a new validation service.
func NewService(exec execution.Service, f txn.Factory) Service {
	return Service{
		execution: exec,
		fac:       NewResultFactory(f),
		hashFac:   crypto.NewSha256Factory(),
	}
}

// GetFactory implements validation.Service. It returns the result factory.
func (s Service) GetFactory() validation.ResultFactory {
	return s.fac
}

// GetNonce implements validation.Service. It reads the latest nonce in the
// storage for the given identity and returns the next valid nonce.
func (s Service) GetNonce(store store.Readable, ident access.Identity) (uint64, error) {
	if ident == nil {
		return 0, xerrors.New("missing identity in transaction")
	}

	key, err := s.keyFromIdentity(ident)
	if err != nil {
		return 0, xerrors.Errorf("key: %v", err)
	}

	value, err := store.Get(key)
	if err != nil {
		return 0, xerrors.Errorf("store: %v", err)
	}

	if value == nil || len(value) != 8 {
		return 0, nil
	}

	return binary.LittleEndian.Uint64(value) + 1, nil
}

// Accept implements validation.Service. It returns nil if the transaction would
// be accepted by the service given some leeway and a snapshot of the storage.
func (s Service) Accept(store store.Readable, tx txn.Transaction, leeway validation.Leeway) error {
	nonce, err := s.GetNonce(store, tx.GetIdentity())
	if err != nil {
		return xerrors.Errorf("while reading nonce: %v", err)
	}

	if tx.GetNonce() < nonce {
		return xerrors.Errorf("nonce '%d' < '%d'", tx.GetNonce(), nonce)
	}

	limit := nonce + uint64(leeway.MaxSequenceDifference)

	if tx.GetNonce() > limit {
		return xerrors.Errorf("nonce '%d' above the limit '%d'", tx.GetNonce(), limit)
	}

	return nil
}

// Validate implements validation.Service. It processes the list of transactions
// while updating the snapshot then returns a bundle of the transaction results.
func (s Service) Validate(store store.Snapshot, txs []txn.Transaction) (validation.Result, error) {
	results := make([]TransactionResult, len(txs))

	step := execution.Step{
		Previous: make([]txn.Transaction, 0, len(txs)),
	}

	for i, tx := range txs {
		res := TransactionResult{tx: tx}

		step.Current = tx

		err := s.validateTx(store, step, &res)
		if err != nil {
			return nil, xerrors.Errorf("tx %#x: %v", tx.GetID()[:4], err)
		}

		if res.accepted {
			step.Previous = append(step.Previous, tx)
		}

		results[i] = res
	}

	res := Result{
		txs: results,
	}

	return res, nil
}

func (s Service) validateTx(store store.Snapshot, step execution.Step, r *TransactionResult) error {
	expectedNonce, err := s.GetNonce(store, step.Current.GetIdentity())
	if err != nil {
		return xerrors.Errorf("nonce: %v", err)
	}

	if expectedNonce != step.Current.GetNonce() {
		r.reason = fmt.Sprintf("nonce is invalid, expected %d, got %d",
			expectedNonce, step.Current.GetNonce())
		r.accepted = false

		return nil
	}

	res, err := s.execution.Execute(store, step)
	// if the execution fail, we don't return an error, but we take it as an
	// invalid transaction.
	if err != nil {
		r.reason = xerrors.Errorf("failed to execute transaction: %v", err).Error()
		r.accepted = false
	} else {
		r.reason = res.Message
		r.accepted = res.Accepted
	}

	// Update the nonce associated to the identity so that this transaction
	// cannot be applied again.
	err = s.set(store, step.Current.GetIdentity(), step.Current.GetNonce())
	if err != nil {
		return xerrors.Errorf("failed to set nonce: %v", err)
	}

	return nil
}

func (s Service) set(store store.Snapshot, ident access.Identity, nonce uint64) error {
	key, err := s.keyFromIdentity(ident)
	if err != nil {
		return xerrors.Errorf("key: %v", err)
	}

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, nonce)

	err = store.Set(key, buffer)
	if err != nil {
		return xerrors.Errorf("store: %v", err)
	}

	return nil
}

func (s Service) keyFromIdentity(ident access.Identity) ([]byte, error) {
	data, err := ident.MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal identity: %v", err)
	}

	h := s.hashFac.New()
	_, err = h.Write(data)
	if err != nil {
		return nil, xerrors.Errorf("failed to write identity: %v", err)
	}

	return h.Sum(nil), nil
}
