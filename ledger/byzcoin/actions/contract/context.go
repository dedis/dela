package contract

import (
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

type actionContext struct {
	basic.Context
	arcFactory arc.AccessControlFactory
	page       inventory.Page
}

// GetArc implements Context. It returns the access control stored in the given
// key if appropriate, otherwise an error.
func (ctx actionContext) GetArc(key []byte) (arc.AccessControl, error) {
	value, err := ctx.page.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read value: %v", err)
	}

	access, err := ctx.arcFactory.FromProto(value)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode access: %v", err)
	}

	return access, nil
}

// Read implements Context. It returns the instance stored at the given key, or
// an error if it does not find it.
func (ctx actionContext) Read(key []byte) (*Instance, error) {
	entry, err := ctx.page.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the value: %v", err)
	}

	instance, ok := entry.(*Instance)
	if !ok {
		return nil, xerrors.Errorf("invalid message type '%T' != '%T'",
			entry, instance)
	}

	return instance, nil
}

// SpawnContext is the context provided to a smart contract execution of a spawn
// transaction.
type SpawnContext struct {
	Context
	SpawnAction
}

// InvokeContext is the context provided to a smart contract execution of an
// invoke transaction.
type InvokeContext struct {
	Context
	InvokeAction
}

// DeleteContext is the context to delete an instance.
type DeleteContext struct {
	Context
	DeleteAction
}
