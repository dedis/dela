package darc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestExpression_GetMatches(t *testing.T) {
	expr := Expression{
		matches: map[string]struct{}{"A": {}, "B": {}},
	}

	require.Len(t, expr.GetMatches(), 2)
}

func TestExpression_Evolve(t *testing.T) {
	expr := newExpression()

	expr, err := expr.Evolve(nil)
	require.NoError(t, err)
	require.Len(t, expr.matches, 0)

	idents := []access.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	expr, err = expr.Evolve(idents)
	require.NoError(t, err)
	require.Len(t, expr.matches, 2)

	expr, err = expr.Evolve(idents)
	require.NoError(t, err)
	require.Len(t, expr.matches, 2)

	_, err = expr.Evolve([]access.Identity{fakeIdentity{err: xerrors.New("oops")}})
	require.EqualError(t, err, "couldn't marshal identity: oops")
}

func TestExpression_Match(t *testing.T) {
	idents := []access.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	expr, err := newExpression().Evolve(idents)
	require.NoError(t, err)

	err = expr.Match(idents)
	require.NoError(t, err)

	err = expr.Match([]access.Identity{fakeIdentity{buffer: []byte{0xcc}}})
	require.EqualError(t, err, "couldn't match identity '\xcc'")

	err = expr.Match([]access.Identity{fakeIdentity{err: xerrors.New("oops")}})
	require.EqualError(t, err, "couldn't marshal identity: oops")
}

func TestExpression_Fingerprint(t *testing.T) {
	expr := Expression{matches: map[string]struct{}{
		"\x01": {},
		"\x03": {},
	}}

	buffer := new(bytes.Buffer)

	err := expr.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x01\x03", buffer.String())

	err = expr.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write match: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeIdentity struct {
	access.Identity
	buffer []byte
	err    error
}

func (i fakeIdentity) MarshalText() ([]byte, error) {
	return i.buffer, i.err
}

func (i fakeIdentity) String() string {
	return string(i.buffer)
}
