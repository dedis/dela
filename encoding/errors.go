package encoding

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
)

type ErrEncoding struct {
	Field string
	Err   error
}

func NewErrEncoding(field string, err error) ErrEncoding {
	return ErrEncoding{
		Field: field,
		Err:   err,
	}
}

func (e ErrEncoding) Error() string {
	return fmt.Sprintf("couldn't encode %s", e.Field)
}

func (e ErrEncoding) Is(err error) bool {
	errenc, ok := err.(ErrEncoding)

	return ok && errenc.Field == e.Field
}

func (e ErrEncoding) Unwrap() error {
	return e.Err
}

type ErrAnyEncoding struct {
	Message proto.Message
	Err     error
}

func NewErrAnyEncoding(msg proto.Message, err error) ErrAnyEncoding {
	return ErrAnyEncoding{
		Message: msg,
		Err:     err,
	}
}

func (e ErrAnyEncoding) Error() string {
	return fmt.Sprintf("couldn't marshal %s to any", e.Message)
}

func (e ErrAnyEncoding) Is(err error) bool {
	errenc, ok := err.(ErrAnyEncoding)

	return ok && reflect.TypeOf(errenc.Message).Kind() == reflect.TypeOf(e.Message).Kind()
}

func (e ErrAnyEncoding) Unwrap() error {
	return e.Err
}
