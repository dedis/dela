package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	signed.RegisterTransactionFormat(serde.FormatJSON, txFormat{})
}

// TransactionJSON is the JSON message of a transaction.
type TransactionJSON struct {
	Nonce     uint64
	Args      map[string][]byte
	PublicKey json.RawMessage
	Signature json.RawMessage
}

// TxFormat is the JSON format engine for transactions.
//
// - implements serde.FormatEngine
type txFormat struct {
	hashFactory crypto.HashFactory
}

// Encode implements serde.FormatEngine. It returns the JSON data of the
// provided transaction if appropriate, otherwise it returns an error.
func (fmt txFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	tx, ok := msg.(*signed.Transaction)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	if tx.GetSignature() == nil {
		return nil, xerrors.New("signature is missing")
	}

	args := map[string][]byte{}
	for _, arg := range tx.GetArgs() {
		args[arg] = tx.GetArg(arg)
	}

	pubkey, err := tx.GetIdentity().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode public key: %v", err)
	}

	sig, err := tx.GetSignature().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode signature: %v", err)
	}

	m := TransactionJSON{
		Nonce:     tx.GetNonce(),
		Args:      args,
		PublicKey: pubkey,
		Signature: sig,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It returns the transaction from the
// JSON data if appropriate, otherwise it returns an error.
func (fmt txFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := TransactionJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	pubkey, err := decodeIdentity(ctx, m.PublicKey)
	if err != nil {
		return nil, xerrors.Errorf("public key: %v", err)
	}

	sig, err := decodeSignature(ctx, m.Signature)
	if err != nil {
		return nil, xerrors.Errorf("signature: %v", err)
	}

	args := make([]signed.TransactionOption, 0, len(m.Args)+2)
	for key, value := range m.Args {
		args = append(args, signed.WithArg(key, value))
	}

	args = append(args, signed.WithSignature(sig))

	if fmt.hashFactory != nil {
		args = append(args, signed.WithHashFactory(fmt.hashFactory))
	}

	tx, err := signed.NewTransaction(m.Nonce, pubkey, args...)
	if err != nil {
		return nil, xerrors.Errorf("failed to create tx: %v", err)
	}

	return tx, nil
}

func decodeIdentity(ctx serde.Context, data []byte) (crypto.PublicKey, error) {
	fac := ctx.GetFactory(signed.PublicKeyFac{})

	factory, ok := fac.(common.PublicKeyFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory '%T'", fac)
	}

	pubkey, err := factory.PublicKeyOf(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("malformed: %v", err)
	}

	return pubkey, nil
}

func decodeSignature(ctx serde.Context, data []byte) (crypto.Signature, error) {
	fac := ctx.GetFactory(signed.SignatureFac{})

	factory, ok := fac.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory '%T'", fac)
	}

	sig, err := factory.SignatureOf(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("malformed: %v", err)
	}

	return sig, nil
}
