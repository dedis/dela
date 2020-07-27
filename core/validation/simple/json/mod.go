package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	simple.RegisterResultFormat(serde.FormatJSON, resFormat{})
	simple.RegisterDataFormat(serde.FormatJSON, dataFormat{})
}

type TransactionResultJSON struct {
	Transaction json.RawMessage
	Accepted    bool
	Reason      string
}

type DataJSON struct {
	Results []json.RawMessage
}

type resFormat struct{}

func (f resFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	txres, ok := msg.(simple.TransactionResult)
	if !ok {
		return nil, xerrors.Errorf("unsupported message")
	}

	tx, err := txres.GetTransaction().Serialize(ctx)
	if err != nil {
		return nil, err
	}

	accepted, reason := txres.GetStatus()

	m := TransactionResultJSON{
		Transaction: tx,
		Accepted:    accepted,
		Reason:      reason,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f resFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := TransactionResultJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	factory := ctx.GetFactory(simple.TransactionKey{})

	fac, ok := factory.(tap.TransactionFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction factory")
	}

	tx, err := fac.TransactionOf(ctx, m.Transaction)
	if err != nil {
		return nil, err
	}

	res := simple.NewTransactionResult(tx, m.Accepted, m.Reason)

	return res, nil
}

type dataFormat struct{}

func (f dataFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	data, ok := msg.(simple.Data)
	if !ok {
		return nil, xerrors.Errorf("unsupported message")
	}

	results := data.GetTransactionResults()
	raws := make([]json.RawMessage, len(results))

	for i, res := range results {
		buffer, err := res.Serialize(ctx)
		if err != nil {
			return nil, err
		}

		raws[i] = buffer
	}

	m := DataJSON{
		Results: raws,
	}

	buffer, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

func (f dataFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := DataJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	factory := ctx.GetFactory(simple.ResultKey{})

	results := make([]simple.TransactionResult, len(m.Results))
	for i, raw := range m.Results {
		msg, err := factory.Deserialize(ctx, raw)
		if err != nil {
			return nil, err
		}

		res, ok := msg.(simple.TransactionResult)
		if !ok {
			return nil, xerrors.Errorf("invalid transaction result")
		}

		results[i] = res
	}

	vdata := simple.NewData(results)

	return vdata, nil
}
