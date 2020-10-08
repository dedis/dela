//
// Documentation Last Review: 12.10.2020
//

package types

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var permFormats = registry.NewSimpleRegistry()

// RegisterPermissionFormat registers the engine for the provided format.
func RegisterPermissionFormat(c serde.Format, f serde.FormatEngine) {
	permFormats.Register(c, f)
}

// DisjunctivePermission is a permission implementation that is using the
// Disjunctive Normal Form to represent the groups of identities allowed for a
// given rule.
//
// - implements types.Permission
type DisjunctivePermission struct {
	rules map[string]*Expression
}

// PermissionOption is the option type to create an access control.
type PermissionOption func(*DisjunctivePermission)

// WithRule is an option to grant a given group access to a rule.
func WithRule(rule string, group ...access.Identity) PermissionOption {
	return func(perm *DisjunctivePermission) {
		perm.Allow(rule, group...)
	}
}

// WithExpression is an option to set a rule from its expression.
func WithExpression(rule string, expr *Expression) PermissionOption {
	return func(perm *DisjunctivePermission) {
		perm.rules[rule] = expr
	}
}

// NewPermission returns a new empty instance of an access control.
func NewPermission(opts ...PermissionOption) *DisjunctivePermission {
	a := &DisjunctivePermission{
		rules: make(map[string]*Expression),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// GetRules returns a map of the expressions.
func (perm *DisjunctivePermission) GetRules() map[string]*Expression {
	rules := make(map[string]*Expression)

	for rule, expr := range perm.rules {
		rules[rule] = expr
	}

	return rules
}

// Allow implements types.Permission. It grants the permission to the group of
// identities as a single entity.
func (perm *DisjunctivePermission) Allow(rule string, group ...access.Identity) {
	expr, ok := perm.rules[rule]
	if !ok {
		expr = NewExpression()
	}

	expr.Allow(group)

	perm.rules[rule] = expr
}

// Deny implements types.Permission. It denies the permission to the group of
// identities as a single entity by removing every subset matching this
// superset.
func (perm *DisjunctivePermission) Deny(rule string, group ...access.Identity) {
	expr, ok := perm.rules[rule]
	if !ok {
		return
	}

	expr.Deny(group)

	// Clean the the rule if it was the last group allowed.
	if len(expr.matches) == 0 {
		delete(perm.rules, rule)
	}
}

// Match implements types.Permission. It returns true if the rule exists and the
// group of identities is associated with it.
func (perm *DisjunctivePermission) Match(rule string, group ...access.Identity) error {
	if len(group) == 0 {
		return xerrors.New("expect at least one identity")
	}

	expr, ok := perm.rules[rule]
	if !ok {
		return xerrors.Errorf("rule '%s' not found", rule)
	}

	err := expr.Match(group)
	if err != nil {
		return xerrors.Errorf("rule '%s': %v", rule, err)
	}

	return nil
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data of the permission.
func (perm *DisjunctivePermission) Serialize(ctx serde.Context) ([]byte, error) {
	format := permFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, perm)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode access: %v", err)
	}

	return data, nil
}

// PublicKeyFac is the key of the public key factory.
type PublicKeyFac struct{}

// permFac is the implementation of a permission factory.
//
// - implements types.PermissionFactory
type permFac struct {
	fac common.PublicKeyFactory
}

// NewFactory returns a new instance of the factory.
func NewFactory() PermissionFactory {
	return permFac{
		fac: common.NewPublicKeyFactory(),
	}
}

// Deserialize implements serde.Factory.
func (f permFac) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.PermissionOf(ctx, data)
}

// PermissionOf implements types.PermissionFactory.
func (f permFac) PermissionOf(ctx serde.Context, data []byte) (Permission, error) {
	format := permFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, PublicKeyFac{}, f.fac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("%v format: %v", ctx.GetFormat(), err)
	}

	access, ok := msg.(Permission)
	if !ok {
		return nil, xerrors.Errorf("invalid access '%T'", msg)
	}

	return access, nil
}
