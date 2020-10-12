// Package types implements the darc messages.
//
// The messages are implemented in a separate package to prevent cycle imports
// when importing the serde formats.
//
// Documentation Last Review: 08.10.2020
//
package types

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/serde"
)

// Permission is the interface of the underlying permissions used by the
// service.
type Permission interface {
	serde.Message

	// Allow grants the permission to the rule to the group of identities as a
	// single entity so that it will match if and only if the group agrees.
	Allow(rule string, group ...access.Identity)

	// Deny denies the permission to the rule to the group of identities as a
	// single entity.
	Deny(rule string, group ...access.Identity)

	// Match returns a nil error if the group, or a subset of the group, is
	// allowed.
	Match(rule string, group ...access.Identity) error
}

// PermissionFactory is the factory to serialize and deserialize the
// permissions.
type PermissionFactory interface {
	serde.Factory

	PermissionOf(serde.Context, []byte) (Permission, error)
}
