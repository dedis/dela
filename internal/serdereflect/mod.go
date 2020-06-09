package serdereflect

import (
	"reflect"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// AssignTo uses the reflection to assign a serde message to a given interface
// if it fits, otherwise it returns an error.
func AssignTo(m serde.Message, o interface{}) error {
	value := reflect.ValueOf(o)
	if value.Kind() != reflect.Ptr {
		return xerrors.New("expect a pointer")
	}

	if !reflect.TypeOf(m).AssignableTo(value.Elem().Type()) {
		return xerrors.New("types are not assignable")
	}

	value.Elem().Set(reflect.ValueOf(m))
	return nil
}
