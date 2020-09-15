package darc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterPermissionFormat(fake.GoodFormat, fake.Format{Msg: &DisjunctivePermission{}})
	RegisterPermissionFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestPermission_WithRule(t *testing.T) {
	perm := NewPermission(WithRule("A", newIdentity("AA"), newIdentity("BB")))

	require.Len(t, perm.rules, 1)
	require.Len(t, perm.rules["A"].matches, 1)
	require.Len(t, perm.rules["A"].matches[0], 2)
}

func TestPermission_GetRules(t *testing.T) {
	perm := NewPermission(WithRule("A", newIdentity("a")), WithRule("B", newIdentity("b")))

	require.Len(t, perm.GetRules(), 2)
}

func TestPermission_Evolve(t *testing.T) {
	perm := NewPermission()

	idents := []access.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	perm.Evolve("fake", true, idents...)
	require.Len(t, perm.rules, 1)

	perm.Evolve("another", true, idents...)
	require.Len(t, perm.rules, 2)

	perm.Evolve("fake", true)
	require.Len(t, perm.rules, 2)

	perm.Evolve("fake", false, idents...)
	require.Len(t, perm.rules, 1)

	perm.Evolve("fake", false, idents...)
	require.Len(t, perm.rules, 1)
}

func TestPermission_Match(t *testing.T) {
	idents := []access.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	perm := NewPermission()
	perm.Evolve("fake", true, idents...)

	err := perm.Match("fake", idents...)
	require.NoError(t, err)

	err = perm.Match("fake")
	require.EqualError(t, err, "expect at least one identity")

	err = perm.Match("unknown", idents...)
	require.EqualError(t, err, "rule 'unknown' not found")

	err = perm.Match("fake", newIdentity("C"))
	require.EqualError(t, err,
		"couldn't match 'fake': unauthorized: ['C']")
}

func TestPermission_Serialize(t *testing.T) {
	perm := NewPermission()

	data, err := perm.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = perm.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode access: fake error")
}

func TestFactory_Deserialize(t *testing.T) {
	factory := NewFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, &DisjunctivePermission{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode access: fake error")
}
