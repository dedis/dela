package darc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc/types"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

var testCtx = json.NewContext()

func TestService_Match(t *testing.T) {
	store := fake.NewSnapshot()

	alice := bls.NewSigner()
	bob := bls.NewSigner()

	creds := access.NewContractCreds([]byte{0xaa}, "test", "match")

	perm := types.NewPermission()
	perm.Allow(creds.GetRule(), alice.GetPublicKey())
	data, err := perm.Serialize(testCtx)
	require.NoError(t, err)

	store.Set([]byte{0xaa}, data)
	store.Set([]byte{0xbb}, []byte{})

	srvc := NewService(testCtx)

	err = srvc.Match(store, creds, alice.GetPublicKey())
	require.NoError(t, err)

	// Only the key of Alice is necessary, so it should pass.
	err = srvc.Match(store, creds, alice.GetPublicKey(), bob.GetPublicKey())
	require.NoError(t, err)

	err = srvc.Match(store, creds, bob.GetPublicKey())
	require.Error(t, err)
	require.Regexp(t,
		"^permission: rule 'test:match': unauthorized: \\[bls:[[:xdigit:]]+\\]", err.Error())

	err = srvc.Match(fake.NewBadSnapshot(), creds, alice.GetPublicKey())
	require.EqualError(t, err, fake.Err("store failed: while reading"))

	err = srvc.Match(store, access.NewContractCreds([]byte{0xcc}, "", ""))
	require.EqualError(t, err, "permission 0xcc not found")

	err = srvc.Match(store, access.NewContractCreds([]byte{0xbb}, "", ""), alice.GetPublicKey())
	require.EqualError(t, err,
		"store failed: permission malformed: JSON format: failed to unmarshal: unexpected end of JSON input")
}

func TestService_Grant(t *testing.T) {
	store := fake.NewSnapshot()
	store.Set([]byte{0xbb}, []byte{})

	creds := access.NewContractCreds([]byte{0xaa}, "test", "grant")

	alice := bls.NewSigner()
	bob := bls.NewSigner()

	srvc := NewService(testCtx)

	err := srvc.Grant(store, creds, alice.GetPublicKey())
	require.NoError(t, err)

	err = srvc.Grant(store, creds, bob.GetPublicKey())
	require.NoError(t, err)

	err = srvc.Grant(fake.NewBadSnapshot(), creds)
	require.EqualError(t, err, fake.Err("store failed: while reading"))

	err = srvc.Grant(store, access.NewContractCreds([]byte{0xbb}, "", ""))
	require.EqualError(t, err,
		"store failed: permission malformed: JSON format: failed to unmarshal: unexpected end of JSON input")

	srvc.fac = badFac{}
	err = srvc.Grant(store, creds, alice.GetPublicKey())
	require.EqualError(t, err, fake.Err("failed to serialize"))

	badStore := fake.NewSnapshot()
	badStore.ErrWrite = fake.GetError()
	err = srvc.Grant(badStore, creds, alice.GetPublicKey())
	require.EqualError(t, err, fake.Err("store failed to write"))
}

// -----------------------------------------------------------------------------
// Utility functions

type badFac struct {
	types.PermissionFactory
}

func (badFac) PermissionOf(serde.Context, []byte) (types.Permission, error) {
	return badPerm{}, nil
}

type badPerm struct {
	types.Permission
}

func (badPerm) Allow(string, ...access.Identity) {}

func (badPerm) Serialize(serde.Context) ([]byte, error) {
	return nil, fake.GetError()
}
