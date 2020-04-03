package darc

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"golang.org/x/xerrors"
)

type expression struct {
	matches map[string]struct{}
}

func newExpression() expression {
	return expression{
		matches: make(map[string]struct{}),
	}
}

func (expr expression) Evolve(targets []arc.Identity) (expression, error) {
	e := expr.Clone()

	for _, target := range targets {
		text, err := target.MarshalText()
		if err != nil {
			return e, err
		}

		e.matches[string(text)] = struct{}{}
	}

	return e, nil
}

func (expr expression) Match(targets []arc.Identity) error {
	for _, target := range targets {
		text, err := target.MarshalText()
		if err != nil {
			return xerrors.Errorf("couldn't marshal identity: %v", err)
		}

		if _, ok := expr.matches[string(text)]; !ok {
			return xerrors.Errorf("couldn't match identity %v", target)
		}
	}

	return nil
}

func (expr expression) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Expression{
		Matches: make([]string, 0, len(expr.matches)),
	}

	for match := range expr.matches {
		pb.Matches = append(pb.Matches, match)
	}

	return pb, nil
}

func (expr expression) Clone() expression {
	e := newExpression()
	for match := range expr.matches {
		e.matches[match] = struct{}{}
	}

	return e
}
