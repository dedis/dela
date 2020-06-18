// Package json defines the JSON messages for the basic transactions.
package json

import (
	"encoding/json"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func init() {
	basic.RegisterTxFormat(serdeng.CodecJSON, txFormat{})
}

// Task is a container of a task with its type and the raw value to deserialize.
type Task struct {
	Type  string
	Value json.RawMessage
}

// Transaction is a combination of a given task and some metadata including the
// nonce to prevent replay attack and the signature to prove the identity of the
// client.
type Transaction struct {
	Nonce     uint64
	Identity  json.RawMessage
	Signature json.RawMessage
	Task      Task
}

type txFormat struct {
	hashFactory crypto.HashFactory
}

func (f txFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	var tx basic.ClientTransaction
	switch in := msg.(type) {
	case basic.ClientTransaction:
		tx = in
	case basic.ServerTransaction:
		tx = in.ClientTransaction
	default:
		return nil, xerrors.New("invalid tx")
	}

	identity, err := tx.GetIdentity().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize identity: %v", err)
	}

	signature, err := tx.GetSignature().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize signature: %v", err)
	}

	task, err := tx.GetTask().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize task: %v", err)
	}

	m := Transaction{
		Nonce:     tx.GetNonce(),
		Identity:  identity,
		Signature: signature,
		Task: Task{
			Type:  basic.KeyOf(tx.GetTask()),
			Value: task,
		},
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f txFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := Transaction{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize transaction: %v", err)
	}

	identity, err := decodeIdentity(ctx, m.Identity)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize identity: %v", err)
	}

	signature, err := decodeSignature(ctx, m.Signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
	}

	task, err := decodeTask(ctx, m.Task.Type, m.Task.Value)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	opts := []basic.ServerTransactionOption{
		basic.WithNonce(m.Nonce),
		basic.WithIdentity(identity, signature),
		basic.WithTask(task),
	}

	if f.hashFactory != nil {
		opts = append(opts, basic.WithHashFactory(f.hashFactory))
	}

	tx, err := basic.NewServerTransaction(opts...)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create tx: %v", err)
	}

	return tx, nil
}

func decodeIdentity(ctx serdeng.Context, data []byte) (crypto.PublicKey, error) {
	factory := ctx.GetFactory(basic.IdentityKey{})

	fac, ok := factory.(crypto.PublicKeyFactory)
	if !ok {
		return nil, xerrors.New("invalid factory")
	}

	pubkey, err := fac.PublicKeyOf(ctx, data)
	if err != nil {
		return nil, err
	}

	return pubkey, nil
}

func decodeSignature(ctx serdeng.Context, data []byte) (crypto.Signature, error) {
	factory := ctx.GetFactory(basic.SignatureKey{})

	fac, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.New("invalid factory")
	}

	sig, err := fac.SignatureOf(ctx, data)
	if err != nil {
		return nil, err
	}

	return sig, nil
}

func decodeTask(ctx serdeng.Context, key string, data []byte) (basic.ServerTask, error) {
	factory := ctx.GetFactory(basic.TaskKey{})

	fac, ok := factory.(basic.TransactionFactory)
	if !ok {
		return nil, xerrors.New("invalid factory")
	}

	taskFac := fac.Get(key)
	if taskFac == nil {
		return nil, xerrors.Errorf("factory '%s' not found", key)
	}

	task, err := taskFac.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return task.(basic.ServerTask), nil
}
