// Package darc implements the Distributed Access Rights Control.
package darc

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Service is an implementation of an access service that will allow one to
// store and verify access for a group of identities.
type Service struct {
	fac     types.PermissionFactory
	context serde.Context
}

// NewService creates a new service.
func NewService(ctx serde.Context) Service {
	return Service{
		fac:     types.NewFactory(),
		context: ctx,
	}
}

// Match implements access.Service. It returns nil if the group of identities
// have access to the given credentials, otherwise a meaningful error on the
// reason it does not have access.
func (srvc Service) Match(store store.Readable, creds access.Credentials, idents ...access.Identity) error {
	perm, err := srvc.readPermission(store, creds.GetID())
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	if perm == nil {
		return xerrors.Errorf("permission %#x not found", creds.GetID())
	}

	err = perm.Match(creds.GetRule(), idents...)
	if err != nil {
		return xerrors.Errorf("permission: %v", err)
	}

	return nil
}

// Grant implements access.Service. It updates or creates the credentials and
// grant the access to the group of identities.
func (srvc Service) Grant(store store.Snapshot, creds access.Credentials, idents ...access.Identity) error {
	perm, err := srvc.readPermission(store, creds.GetID())
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	if perm == nil {
		perm = types.NewPermission()
	}

	perm.Allow(creds.GetRule(), idents...)

	value, err := perm.Serialize(srvc.context)
	if err != nil {
		return xerrors.Errorf("failed to serialize: %v", err)
	}

	err = store.Set(creds.GetID(), value)
	if err != nil {
		return xerrors.Errorf("store failed to write: %v", err)
	}

	return nil
}

func (srvc Service) readPermission(store store.Readable, key []byte) (types.Permission, error) {
	value, err := store.Get(key)
	if err != nil {
		return nil, xerrors.Errorf("while reading: %v", err)
	}

	if value == nil {
		return nil, nil
	}

	perm, err := srvc.fac.PermissionOf(srvc.context, value)
	if err != nil {
		return nil, xerrors.Errorf("permission malformed: %v", err)
	}

	return perm, nil
}
