package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	darc.RegisterPermissionFormat(serde.FormatJSON, permFormat{})
}

type PermissionJSON struct {
	Expressions map[string]json.RawMessage
}

type ExpressionJSON struct {
	Identities []json.RawMessage
	Matches    [][]int
}

type permFormat struct{}

func (permFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	perm, ok := msg.(*darc.DisjunctivePermission)
	if !ok {
		return nil, xerrors.Errorf("invalid permission '%T'", msg)
	}

	expressions := make(map[string]json.RawMessage)

	for key, expr := range perm.GetRules() {
		data, err := encodeExpression(ctx, expr)
		if err != nil {
			return nil, err
		}

		expressions[key] = data
	}

	m := PermissionJSON{
		Expressions: expressions,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func encodeExpression(ctx serde.Context, expr *darc.Expression) ([]byte, error) {
	identities := make(darc.IdentitySet, 0)

	matches := make([][]int, len(expr.GetIdentitySets()))

	for i, match := range expr.GetIdentitySets() {
		indices := make([]int, len(match))

		for j, ident := range match {
			index, found := identities.Search(ident)
			if !found {
				identities = append(identities, ident)
				index = len(identities) - 1
			}

			indices[j] = index
		}

		matches[i] = indices
	}

	identitiesRaw := make([]json.RawMessage, len(identities))

	for i, ident := range identities {
		data, err := ident.Serialize(ctx)
		if err != nil {
			return nil, err
		}

		identitiesRaw[i] = data
	}

	m := ExpressionJSON{
		Identities: identitiesRaw,
		Matches:    matches,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (permFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := PermissionJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	opts := make([]darc.PermissionOption, 0, len(m.Expressions))

	for rule, raw := range m.Expressions {
		expr, err := decodeExpr(ctx, raw)
		if err != nil {
			return nil, err
		}

		opts = append(opts, darc.WithExpression(rule, expr))
	}

	return darc.NewPermission(opts...), nil
}

func decodeExpr(ctx serde.Context, data []byte) (*darc.Expression, error) {
	m := ExpressionJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	fac := ctx.GetFactory(darc.PublicKeyFacKey{})

	factory, ok := fac.(common.PublicKeyFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid public key factory '%T'", fac)
	}

	identities := make([]access.Identity, len(m.Identities))

	for i, raw := range m.Identities {
		pubkey, err := factory.PublicKeyOf(ctx, raw)
		if err != nil {
			return nil, err
		}

		identities[i] = pubkey
	}

	matches := make([]darc.IdentitySet, len(m.Matches))

	for i, indices := range m.Matches {
		matches[i] = make(darc.IdentitySet, len(indices))

		for j, index := range indices {
			matches[i][j] = identities[index]
		}
	}

	return darc.NewExpression(matches...), nil
}
