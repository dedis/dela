// Package darc implements the Distributed Access Rights Control.
package darc

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/serde"
)

// Permission is the interface of the underlying permissions used by the
// service.
type Permission interface {
	serde.Message

	Evolve(rule string, grant bool, group ...access.Identity)

	Match(rule string, group ...access.Identity) error
}

// PermissionFactory is the factory to serialize and deserialize the
// permissions.
type PermissionFactory interface {
	serde.Factory

	PermissionOf(serde.Context, []byte) (Permission, error)
}

// Service is an implementation of an access service that will allow one to
// store and verify access for a group of identities.
type Service struct {
	fac     PermissionFactory
	context serde.Context
}

// NewService creates a new service.
func NewService(ctx serde.Context) Service {
	return Service{
		fac:     NewFactory(),
		context: ctx,
	}
}

// Match implements access.Service. It returns nil if the group of identities
// have access to the given credentials, otherwise a meaningful error on the
// reason it does not have access.
func (srvc Service) Match(store store.Readable, creds access.Credentials, idents ...access.Identity) error {
	value, err := store.Get(creds.GetID())
	if err != nil {
		return err
	}

	perm, err := srvc.fac.PermissionOf(srvc.context, value)
	if err != nil {
		return err
	}

	err = perm.Match(creds.GetRule(), idents...)
	if err != nil {
		return err
	}

	return nil
}

// Grant implements access.Service. It updates or create the credentials and
// grant the access to the group of identities.
func (srvc Service) Grant(store store.Snapshot, creds access.Credentials, idents ...access.Identity) error {
	value, err := store.Get(creds.GetID())
	if err != nil {
		return err
	}

	var perm Permission
	if value == nil {
		perm = NewPermission()
	} else {
		perm, err = srvc.fac.PermissionOf(srvc.context, value)
		if err != nil {
			return err
		}
	}

	perm.Evolve(creds.GetRule(), true, idents...)

	value, err = perm.Serialize(srvc.context)
	if err != nil {
		return err
	}

	err = store.Set(creds.GetID(), value)
	if err != nil {
		return err
	}

	return nil
}
