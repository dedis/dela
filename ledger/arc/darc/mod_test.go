package darc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"golang.org/x/xerrors"
)

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
