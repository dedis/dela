package darc

import (
	"io"
	"sort"

	"go.dedis.ch/dela/ledger/arc"
	"golang.org/x/xerrors"
)

// Expression is an abstraction of a list of identities allowed for a given
// rule.
//
// - implements encoding.Packable
type Expression struct {
	matches map[string]struct{}
}

func newExpression() Expression {
	return Expression{
		matches: make(map[string]struct{}),
	}
}

// GetMatches returns the list of possible matches for an expression.
func (expr Expression) GetMatches() []string {
	matches := make([]string, 0, len(expr.matches))
	for match := range expr.matches {
		matches = append(matches, match)
	}

	return matches
}

// Evolve returns a new expression with the targets added in the list of
// authorized identities.
func (expr Expression) Evolve(targets []arc.Identity) (Expression, error) {
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
func (expr Expression) Match(targets []arc.Identity) error {
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
func (expr Expression) Fingerprint(w io.Writer) error {
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

// Clone returns a deep copy of the expression.
func (expr Expression) Clone() Expression {
	e := newExpression()
	for match := range expr.matches {
		e.matches[match] = struct{}{}
	}

	return e
}
