// Package darc implements the Distributed Access Rights Control.
package darc

import (
	"io"
	"sort"

	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var accessFormats = registry.NewSimpleRegistry()

// RegisterAccessFormat registers the engine for the provided format.
func RegisterAccessFormat(c serde.Format, f serde.FormatEngine) {
	accessFormats.Register(c, f)
}

// Access is the DARC implementation of an Evolvable Access Control.
//
// - implements darc.EvolvableAccessControl
// - implements encoding.Packable
type Access struct {
	rules map[string]Expression
}

// AccessOption is the option type to create an access control.
type AccessOption func(*Access)

// WithRule is an option to set a rule to a new access control.
func WithRule(rule string, matches []string) AccessOption {
	return func(a *Access) {
		mapper := make(map[string]struct{})
		for _, match := range matches {
			mapper[match] = struct{}{}
		}

		a.rules[rule] = Expression{matches: mapper}
	}
}

// NewAccess returns a new empty instance of an access control.
func NewAccess(opts ...AccessOption) Access {
	a := Access{
		rules: make(map[string]Expression),
	}

	for _, opt := range opts {
		opt(&a)
	}

	return a
}

// GetRules returns the list of rules of an access control.
func (ac Access) GetRules() map[string]Expression {
	rules := make(map[string]Expression)
	for k, v := range ac.rules {
		rules[k] = v
	}

	return rules
}

// Evolve implements darc.EvolvableAccessControl. It updates the rule with the
// list of targets.
func (ac Access) Evolve(rule string, targets ...arc.Identity) (Access, error) {
	access := ac.Clone()

	expr, ok := access.rules[rule]
	if !ok {
		expr = newExpression()
	}

	expr, err := expr.Evolve(targets)
	if err != nil {
		return access, xerrors.Errorf("couldn't evolve rule: %v", err)
	}

	access.rules[rule] = expr

	return access, nil
}

// Match implements arc.AccessControl. It returns true if the rule exists and
// the identity is associated with it.
func (ac Access) Match(rule string, targets ...arc.Identity) error {
	if len(targets) == 0 {
		return xerrors.New("expect at least one identity")
	}

	expr, ok := ac.rules[rule]
	if !ok {
		return xerrors.Errorf("rule '%s' not found", rule)
	}

	err := expr.Match(targets)
	if err != nil {
		return xerrors.Errorf("couldn't match '%s': %v", rule, err)
	}

	return nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the access to
// the writer in a deterministic way.
func (ac Access) Fingerprint(w io.Writer) error {
	keys := make(sort.StringSlice, 0, len(ac.rules))
	for key := range ac.rules {
		keys = append(keys, key)
	}

	sort.Sort(keys)

	for _, key := range keys {
		_, err := w.Write([]byte(key))
		if err != nil {
			return xerrors.Errorf("couldn't write key: %v", err)
		}

		err = ac.rules[key].Fingerprint(w)
		if err != nil {
			return xerrors.Errorf("couldn't fingerprint rule '%s': %v", key, err)
		}
	}

	return nil
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the access.
func (ac Access) Serialize(ctx serde.Context) ([]byte, error) {
	format := accessFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, ac)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode access: %v", err)
	}

	return data, nil
}

// Clone returns a deep copy of the access control.
func (ac Access) Clone() Access {
	access := Access{rules: make(map[string]Expression)}
	for rule, expr := range ac.rules {
		access.rules[rule] = expr.Clone()
	}

	return access
}

// Factory is the implementation of an access control factory for DARCs.
type Factory struct{}

// NewFactory returns a new instance of the factory.
func NewFactory() Factory {
	return Factory{}
}

// Deserialize implements serde.Factory.
func (f Factory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := accessFormats.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode access: %v", err)
	}

	return msg, nil
}
