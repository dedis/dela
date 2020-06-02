package gob

import (
	"bytes"
	"encoding/gob"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

type gobWrapper struct {
	Type  string
	Value []byte
}

type gobEncoder struct{}

func (e gobEncoder) Encode(m interface{}) ([]byte, error) {
	buffer := new(bytes.Buffer)

	enc := gob.NewEncoder(buffer)
	err := enc.Encode(m)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (e gobEncoder) Decode(data []byte, m interface{}) error {
	buffer := bytes.NewBuffer(data)

	enc := gob.NewDecoder(buffer)
	err := enc.Decode(m)
	if err != nil {
		return err
	}

	return nil
}

func (e gobEncoder) Wrap(m interface{}) (serde.Raw, error) {
	buffer, err := e.Encode(m)
	if err != nil {
		return nil, err
	}

	msg := gobWrapper{
		Type:  serde.KeyOf(m),
		Value: buffer,
	}

	msgBuffer, err := e.Encode(msg)
	if err != nil {
		return nil, err
	}

	return msgBuffer, nil
}

func (e gobEncoder) Unwrap(raw serde.Raw) (interface{}, error) {
	msg := gobWrapper{}
	err := e.Decode(raw, &msg)
	if err != nil {
		return nil, err
	}

	value, found := serde.New(msg.Type)
	if !found {
		return nil, xerrors.New("oops")
	}

	err = e.Decode(msg.Value, value)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (e gobEncoder) MessageOf(p serde.Packable) (interface{}, error) {
	return p.Pack(e)
}
