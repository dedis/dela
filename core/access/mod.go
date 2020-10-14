// Package access defines the interfaces for Access Rights Controls.
//
// Documentation Last Review: 08.10.2020
//
package access

import (
	"encoding"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/serde"
)

// Identity is an abstraction to uniquely identify a signer.
type Identity interface {
	serde.Message

	encoding.TextMarshaler

	// Equal returns true when the other object is equal to the identity.
	Equal(other interface{}) bool
}

// Credential is an abstraction of an entity that allows one or several
// identities to access a given scope.
//
// As an example, the identifier is the username of a username/password pair. It
// defines the component to compare against. Then the password is the list of
// identities, verified beforehands, that will match, or won't, match to the
// identifier underlying permissions. The rule defines which scope should be
// verified so that the permissions can hold multiple of thoses.
//
//   -- 0xdeadbeef
//      -- "myContract:sayHello"
//         -- Alice
//         -- Bob
//      -- "myContract:sayBye"
//         -- Bob
//
// The example above shows two credentials for the contract "myContract" that is
// allowing two commands "sayHello" and "sayBye". Alice and Bob can say hello,
// but only Bob is allow to say bye. Alice can prove that she's allowed by
// providing the credential with the identifier 0xdeadbeef and the rule
// "myContract:sayHello".
type Credential interface {
	// GetID returns the identifier of the credential.
	GetID() []byte

	// GetRule returns the rule that is targetted by the credential.
	GetRule() string
}

// Service is an access control service that can read the storage to find
// permissions associated to the credentials, or update existing ones.
type Service interface {
	// Match returns nil if the credentials can be matched to the group of
	// identities.
	Match(store store.Readable, creds Credential, idents ...Identity) error

	// Grant updates the store so that the group of identities will match the
	// credentials.
	Grant(store store.Snapshot, creds Credential, idents ...Identity) error
}
