package darc

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
)

func TestIdentitySet_New(t *testing.T) {
	iset := NewIdentitySet(newIdentity("A"), newIdentity("B"), newIdentity("A"))
	require.Len(t, iset, 2)
}

func TestIdentitySet_Search(t *testing.T) {
	iset := NewIdentitySet(newIdentity("A"), newIdentity("B"))

	require.True(t, iset.Contains(newIdentity("A")))
	require.True(t, iset.Contains(newIdentity("B")))
	require.False(t, iset.Contains(newIdentity("C")))
}

func TestIdentitySet_Equal(t *testing.T) {
	iset := NewIdentitySet(newIdentity("A"), newIdentity("C"))

	require.True(t, iset.Equal(iset))
	require.False(t, iset.Equal(NewIdentitySet()))
	require.False(t, iset.Equal(NewIdentitySet(newIdentity("A"), newIdentity("B"))))
}

func TestExpression_GetIdentitySets(t *testing.T) {
	expr := Expression{
		matches: []IdentitySet{{}, {}},
	}

	require.Len(t, expr.GetIdentitySets(), 2)
}

func TestExpression_Evolve(t *testing.T) {
	expr := NewExpression()

	expr.Evolve(true, nil)
	require.Len(t, expr.matches, 0)

	idents := []access.Identity{newIdentity("A"), newIdentity("B")}

	expr.Evolve(true, idents)
	require.Len(t, expr.matches, 1)
	require.Len(t, expr.matches[0], 2)

	expr.Evolve(true, idents)
	require.Len(t, expr.matches, 1)

	expr.Evolve(true, []access.Identity{newIdentity("A"), newIdentity("C")})
	require.Len(t, expr.matches, 2)

	expr.Evolve(false, idents)
	require.Len(t, expr.matches, 1)
}

func TestExpression_Match(t *testing.T) {
	idents := []access.Identity{newIdentity("A"), newIdentity("B")}

	expr := NewExpression()
	expr.Evolve(true, idents)

	err := expr.Match(idents)
	require.NoError(t, err)

	err = expr.Match([]access.Identity{newIdentity("A"), newIdentity("C")})
	require.EqualError(t, err, "unauthorized: ['A' 'C']")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeIdentity struct {
	access.Identity

	buffer []byte
}

func newIdentity(value string) fakeIdentity {
	return fakeIdentity{buffer: []byte(value)}
}

func (i fakeIdentity) String() string {
	return fmt.Sprintf("'%s'", i.buffer)
}

func (i fakeIdentity) Equal(o interface{}) bool {
	other, ok := o.(fakeIdentity)
	return ok && bytes.Equal(i.buffer, other.buffer)
}
