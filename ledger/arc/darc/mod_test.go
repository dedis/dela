package darc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"golang.org/x/xerrors"
)

func init() {
	RegisterAccessFormat(fake.GoodFormat, fake.Format{Msg: Access{}})
	RegisterAccessFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestAccess_WithRule(t *testing.T) {
	access := NewAccess(WithRule("A", []string{"B", "C"}))

	require.Len(t, access.rules, 1)
	require.Len(t, access.rules["A"].matches, 2)
}

func TestAccess_GetRules(t *testing.T) {
	access := NewAccess(WithRule("A", nil), WithRule("B", nil))

	require.Len(t, access.GetRules(), 2)
}

func TestAccess_Evolve(t *testing.T) {
	access := NewAccess()

	idents := []arc.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	access, err := access.Evolve("fake", idents...)
	require.NoError(t, err)
	require.Len(t, access.rules, 1)

	access, err = access.Evolve("another", idents...)
	require.NoError(t, err)
	require.Len(t, access.rules, 2)

	access, err = access.Evolve("fake")
	require.NoError(t, err)
	require.Len(t, access.rules, 2)

	_, err = access.Evolve("fake", fakeIdentity{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't evolve rule: couldn't marshal identity: oops")
}

func TestAccess_Match(t *testing.T) {
	idents := []arc.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	access, err := NewAccess().Evolve("fake", idents...)
	require.NoError(t, err)

	err = access.Match("fake", idents...)
	require.NoError(t, err)

	err = access.Match("fake")
	require.EqualError(t, err, "expect at least one identity")

	err = access.Match("unknown", idents...)
	require.EqualError(t, err, "rule 'unknown' not found")

	err = access.Match("fake", fakeIdentity{buffer: []byte{0xcc}})
	require.EqualError(t, err,
		"couldn't match 'fake': couldn't match identity '\xcc'")
}

func TestAccess_Fingerprint(t *testing.T) {
	access := Access{
		rules: map[string]Expression{
			"\x02": {matches: map[string]struct{}{"\x04": {}}},
		},
	}

	buffer := new(bytes.Buffer)

	err := access.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x02\x04", buffer.String())

	err = access.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write key: fake error")

	err = access.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err,
		"couldn't fingerprint rule '\x02': couldn't write match: fake error")
}

func TestAccess_Serialize(t *testing.T) {
	access := NewAccess()

	data, err := access.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = access.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode access: fake error")
}

func TestFactory_Deserialize(t *testing.T) {
	factory := NewFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, Access{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode access: fake error")
}
