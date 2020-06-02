package encoder

import (
	"encoding/json"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// jsonWrapper is the wrapper to allow a message to decode by itself.
type jsonWrapper struct {
	Type  string
	Value json.RawMessage
}

// JsonEncoder is an encoder using JSON as the underlying format.
type jsonEncoder struct{}

func (e jsonEncoder) Encode(m interface{}) ([]byte, error) {
	return json.Marshal(m)
}

func (e jsonEncoder) Decode(buffer []byte, m interface{}) error {
	return json.Unmarshal(buffer, m)
}

func (e jsonEncoder) Wrap(m interface{}) (serde.Raw, error) {
	buffer, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	typ := serde.KeyOf(m)

	msg := jsonWrapper{
		Type:  typ,
		Value: buffer,
	}

	msgBuffer, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return msgBuffer, nil
}

func (e jsonEncoder) Unwrap(m serde.Raw) (interface{}, error) {
	wrapper := jsonWrapper{}
	err := json.Unmarshal(m, &wrapper)
	if err != nil {
		return nil, err
	}

	value, found := serde.New(wrapper.Type)
	if !found {
		return nil, xerrors.New("oops")
	}

	err = json.Unmarshal(wrapper.Value, value)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (e jsonEncoder) MessageOf(p serde.Packable) (interface{}, error) {
	return p.Pack(e)
}
