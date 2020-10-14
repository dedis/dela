// This file contains the implementation of a dependency injector using
// reflection.
//
// Documentation Last Review: 13.10.2020
//

package node

import (
	"reflect"

	"golang.org/x/xerrors"
)

// ReflectInjector is a dependency injector that uses reflection to resolve
// specific interfaces.
//
// - implements node.Injector
type reflectInjector struct {
	mapper map[reflect.Type]interface{}
}

// NewInjector returns a empty injector.
func NewInjector() Injector {
	return &reflectInjector{
		mapper: make(map[reflect.Type]interface{}),
	}
}

// Resolve implements node.Injector. It populates the given interface with the
// first compatible dependency.
func (inj *reflectInjector) Resolve(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return xerrors.New("expect a pointer")
	}

	if !rv.Elem().IsValid() {
		return xerrors.Errorf("reflect value '%v' is invalid", rv)
	}

	for typ, value := range inj.mapper {
		if typ.AssignableTo(rv.Elem().Type()) {
			rv.Elem().Set(reflect.ValueOf(value))
			return nil
		}
	}

	return xerrors.Errorf("couldn't find dependency for '%v'", rv.Elem().Type())
}

// Inject implements node.Injector. It injects the dependency to be available
// later on.
func (inj *reflectInjector) Inject(v interface{}) {
	key := reflect.TypeOf(v)
	inj.mapper[key] = v
}
