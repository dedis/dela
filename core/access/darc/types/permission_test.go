package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterPermissionFormat(fake.GoodFormat, fake.Format{Msg: &DisjunctivePermission{}})
	RegisterPermissionFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterPermissionFormat(fake.MsgFormat, fake.NewMsgFormat())
}

func TestPermission_WithRule(t *testing.T) {
	perm := NewPermission(WithRule("A", newIdentity("AA"), newIdentity("BB")))

	require.Len(t, perm.rules, 1)
	require.Len(t, perm.rules["A"].matches, 1)
	require.Len(t, perm.rules["A"].matches[0], 2)
}

func TestPermission_WithExpression(t *testing.T) {
	perm := NewPermission(WithExpression("test", NewExpression()))
	require.Len(t, perm.rules, 1)
}

func TestPermission_GetRules(t *testing.T) {
	perm := NewPermission(WithRule("A", newIdentity("a")), WithRule("B", newIdentity("b")))

	require.Len(t, perm.GetRules(), 2)
}

func TestPermission_Allow(t *testing.T) {
	perm := NewPermission()

	idents := []access.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	perm.Allow("fake", idents...)
	require.Len(t, perm.rules, 1)

	perm.Allow("another", idents...)
	require.Len(t, perm.rules, 2)

	perm.Allow("fake")
	require.Len(t, perm.rules, 2)

	perm.Deny("fake", idents...)
	require.Len(t, perm.rules, 1)

	perm.Deny("fake", idents...)
	require.Len(t, perm.rules, 1)
}

func TestPermission_Deny(t *testing.T) {
	perm := NewPermission()
	perm.rules["fake"] = NewExpression(NewIdentitySet(newIdentity("A")))

	perm.Deny("fake")
	require.Len(t, perm.rules, 1)

	perm.Deny("fake", newIdentity("B"))
	require.Len(t, perm.rules, 1)

	perm.Deny("fake", newIdentity("A"))
	require.Len(t, perm.rules, 0)
}

func TestPermission_Match(t *testing.T) {
	idents := []access.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	perm := NewPermission()
	perm.Allow("fake", idents...)

	err := perm.Match("fake", idents...)
	require.NoError(t, err)

	err = perm.Match("fake")
	require.EqualError(t, err, "expect at least one identity")

	err = perm.Match("unknown", idents...)
	require.EqualError(t, err, "rule 'unknown' not found")

	err = perm.Match("fake", newIdentity("C"))
	require.EqualError(t, err, "rule 'fake': unauthorized: ['C']")
}

func TestPermission_Serialize(t *testing.T) {
	perm := NewPermission()

	data, err := perm.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = perm.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode access"))
}

func TestPermissionFactory_Deserialize(t *testing.T) {
	factory := NewFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, &DisjunctivePermission{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("FakeBad format"))

	_, err = factory.Deserialize(fake.NewMsgContext(), nil)
	require.EqualError(t, err, "invalid access 'fake.Message'")
}
