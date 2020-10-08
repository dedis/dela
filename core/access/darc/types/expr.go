//
// Documentation Last Review: 12.10.2020
//

package types

import (
	"go.dedis.ch/dela/core/access"
	"golang.org/x/xerrors"
)

// IdentitySet is a set of identities that belongs to one of the conjunction.
type IdentitySet []access.Identity

// NewIdentitySet creates a new identity set from the list of identities by
// removing duplicates.
func NewIdentitySet(idents ...access.Identity) IdentitySet {
	set := make(IdentitySet, 0, len(idents))
	if len(idents) == 0 {
		return set
	}

	for _, ident := range idents {
		if !set.Contains(ident) {
			set = append(set, ident)
		}
	}

	return set[:]
}

// Contains returns true if the identity exists in the set.
func (set IdentitySet) Contains(target access.Identity) bool {
	_, found := set.Search(target)
	return found
}

// Search searches for the target in the set and returns the index if it exists,
// otherwise a negative value.
func (set IdentitySet) Search(target access.Identity) (int, bool) {
	for i, ident := range set {
		if ident.Equal(target) {
			return i, true
		}
	}

	return -1, false
}

// IsSuperset return true if both sets are the same.
func (set IdentitySet) IsSuperset(o IdentitySet) bool {
	if len(set) < len(o) {
		return false
	}

	for _, ident := range o {
		if !set.Contains(ident) {
			return false
		}
	}

	return true
}

// Expression is the representation of the disjunctive normal form of the
// allowed groups of identities.
type Expression struct {
	matches []IdentitySet
}

// NewExpression creates a new expression from the list of identity sets.
func NewExpression(sets ...IdentitySet) *Expression {
	return &Expression{
		matches: sets,
	}
}

// GetIdentitySets returns the list of identity sets.
func (expr *Expression) GetIdentitySets() []IdentitySet {
	return append([]IdentitySet{}, expr.matches...)
}

// Allow adds the group of identities as long as there is no duplicate.
func (expr *Expression) Allow(group []access.Identity) {
	iset := NewIdentitySet(group...)
	if len(iset) == 0 {
		return
	}

	for _, match := range expr.matches {
		if match.IsSuperset(iset) {
			// The group is already allowed.
			return
		}
	}

	expr.matches = append(expr.matches, iset)
}

// Deny removes the group of identities from the list of matching subsets.
func (expr *Expression) Deny(group []access.Identity) {
	iset := NewIdentitySet(group...)
	if len(iset) == 0 {
		return
	}

	for i, match := range expr.matches {
		if iset.IsSuperset(match) {
			expr.matches = append(expr.matches[:i], expr.matches[i+1:]...)
		}
	}
}

// Match returns nil if the group are allowed for the rule, otherwise it returns
// the reason why it failed.
func (expr *Expression) Match(group []access.Identity) error {
	iset := NewIdentitySet(group...)

	for _, match := range expr.matches {
		if iset.IsSuperset(match) {
			return nil
		}
	}

	return xerrors.Errorf("unauthorized: %v", group)
}
