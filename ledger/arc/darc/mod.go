// Package darc implements the Distributed Access Rights Control.
package darc

import (
	"io"
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/arc/darc/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Access is the DARC implementation of an Evolvable Access Control.
//
// - implements darc.EvolvableAccessControl
// - implements encoding.Packable
type Access struct {
	serde.UnimplementedMessage

	rules map[string]expression
}

// NewAccess returns a new empty instance of an access control.
func NewAccess() Access {
	return Access{
		rules: make(map[string]expression),
	}
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
func (ac Access) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
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

		err = ac.rules[key].Fingerprint(w, e)
		if err != nil {
			return xerrors.Errorf("couldn't fingerprint rule '%s': %v", key, err)
		}
	}

	return nil
}

// Pack implements encoding.Packable.
func (ac Access) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &AccessProto{
		Rules: make(map[string]*Expression),
	}

	for rule, expr := range ac.rules {
		exprpb, err := enc.Pack(expr)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack expression: %v", err)
		}

		pb.Rules[rule] = exprpb.(*Expression)
	}

	return pb, nil
}

// VisitJSON implements serde.Message. It returns the JSON message for the
// access.
func (ac Access) VisitJSON(serde.Serializer) (interface{}, error) {
	rules := make(map[string][]string)
	for key, expr := range ac.rules {
		matches := make([]string, 0, len(expr.matches))
		for m := range expr.matches {
			matches = append(matches, m)
		}

		rules[key] = matches
	}

	m := json.Access{
		Rules: rules,
	}

	return m, nil
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
type Factory struct {
	serde.UnimplementedFactory

	encoder encoding.ProtoMarshaler
}

// NewFactory returns a new instance of the factory.
func NewFactory() Factory {
	return Factory{
		encoder: encoding.NewProtoEncoder(),
	}
}

// FromProto implements arc.AccessControlFactory. It returns the access control
// associated with the protobuf message.
func (f Factory) FromProto(in proto.Message) (arc.AccessControl, error) {
	var pb *AccessProto
	switch msg := in.(type) {
	case *any.Any:
		pb = &AccessProto{}
		err := f.encoder.UnmarshalAny(msg, pb)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
		}
	case *AccessProto:
		pb = msg
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	ac := NewAccess()

	for rule, exprpb := range pb.GetRules() {
		expr := newExpression()
		for _, match := range exprpb.GetMatches() {
			expr.matches[match] = struct{}{}
		}

		ac.rules[rule] = expr
	}

	return ac, nil
}

// VisitJSON implements serde.Factory. It deserializes the access control in
// JSON format.
func (f Factory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Access{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	rules := make(map[string]expression)
	for rule, matches := range m.Rules {
		expr := expression{matches: make(map[string]struct{})}
		for _, m := range matches {
			expr.matches[m] = struct{}{}
		}

		rules[rule] = expr
	}

	return Access{rules: rules}, nil
}
