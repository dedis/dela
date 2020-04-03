// Package darc implements the Distributed Access Rights Control.
package darc

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Access is the implementation for DARC of Access Control.
type Access struct {
	rules map[string]expression
}

// newAccessControl returns a new empty instance of an access control.
func newAccessControl() Access {
	return Access{
		rules: make(map[string]expression),
	}
}

// Evolve updates the rule with the list of targets.
func (ac Access) Evolve(rule string, targets ...arc.Identity) (Access, error) {
	access := ac.Clone()

	expr, ok := access.rules[rule]
	if !ok {
		expr = newExpression()
	}

	expr, err := expr.Evolve(targets)
	if err != nil {
		return access, err
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

	return expr.Match(targets)
}

// Pack implements encoding.Packable.
func (ac Access) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &AccessControlProto{
		Rules: make(map[string]*Expression),
	}

	for rule, expr := range ac.rules {
		exprpb, _ := expr.Pack(enc)

		pb.Rules[rule] = exprpb.(*Expression)
	}

	return pb, nil
}

// Clone returns a deep copy of the access control.
func (ac Access) Clone() Access {
	access := Access{rules: make(map[string]expression)}
	for rule, expr := range ac.rules {
		access.rules[rule] = expr.Clone()
	}

	return access
}

// Factory is the implementation of an access control factory for DARCs.
type Factory struct{}

// FromProto implements arc.AccessControlFactory. It returns the access control
// associated with the protobuf message.
func (f Factory) FromProto(in proto.Message) (arc.AccessControl, error) {
	pb := in.(*AccessControlProto)

	ac := newAccessControl()

	for rule, exprpb := range pb.GetRules() {
		expr := newExpression()
		for _, match := range exprpb.GetMatches() {
			expr.matches[match] = struct{}{}
		}

		ac.rules[rule] = expr
	}

	return ac, nil
}
