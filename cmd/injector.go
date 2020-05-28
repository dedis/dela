package cmd

import (
	"reflect"

	"golang.org/x/xerrors"
)

type reflectInjector struct {
	mapper map[reflect.Type]interface{}
}

// NewInjector returns a empty injector.
func NewInjector() Injector {
	return &reflectInjector{
		mapper: make(map[reflect.Type]interface{}),
	}
}

func (inj *reflectInjector) Resolve(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return xerrors.New("expect a pointer")
	}

	for typ, value := range inj.mapper {
		if typ.AssignableTo(rv.Elem().Type()) {
			rv.Elem().Set(reflect.ValueOf(value))
			return nil
		}
	}

	return xerrors.Errorf("couldn't find dependency for '%v'", rv.Elem().Type())
}

func (inj *reflectInjector) Inject(v interface{}) {
	key := reflect.TypeOf(v)
	inj.mapper[key] = v
}
