package darc

import (
	"io"
	"sort"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"golang.org/x/xerrors"
)

// expression is an abstraction of a list of identities allowed for a given
// rule.
//
// - implements encoding.Packable
type expression struct {
	matches map[string]struct{}
}

func newExpression() expression {
	return expression{
		matches: make(map[string]struct{}),
	}
}

// Evolve returns a new expression with the targets added in the list of
// authorized identities.
func (expr expression) Evolve(targets []arc.Identity) (expression, error) {
	e := expr.Clone()

	for _, target := range targets {
		text, err := target.MarshalText()
		if err != nil {
			return e, xerrors.Errorf("couldn't marshal identity: %v", err)
		}

		e.matches[string(text)] = struct{}{}
	}

	return e, nil
}

// Match returns nil if all the targets are allowed for the rule, otherwise it
// returns the reason why it failed.
func (expr expression) Match(targets []arc.Identity) error {
	for _, target := range targets {
		text, err := target.MarshalText()
		if err != nil {
			return xerrors.Errorf("couldn't marshal identity: %v", err)
		}

		_, ok := expr.matches[string(text)]
		if !ok {
			return xerrors.Errorf("couldn't match identity '%v'", target)
		}
	}

	return nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the expression
// into the writer in a deterministic way.
func (expr expression) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
	matches := make(sort.StringSlice, 0, len(expr.matches))
	for key := range expr.matches {
		matches = append(matches, key)
	}

	sort.Sort(matches)

	for _, match := range matches {
		_, err := w.Write([]byte(match))
		if err != nil {
			return xerrors.Errorf("couldn't write match: %v", err)
		}
	}

	return nil
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// expression.
func (expr expression) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Expression{
		Matches: make([]string, 0, len(expr.matches)),
	}

	for match := range expr.matches {
		pb.Matches = append(pb.Matches, match)
	}

	return pb, nil
}

// Clone returns a deep copy of the expression.
func (expr expression) Clone() expression {
	e := newExpression()
	for match := range expr.matches {
		e.matches[match] = struct{}{}
	}

	return e
}
