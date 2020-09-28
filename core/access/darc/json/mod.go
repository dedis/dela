package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc/types"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterPermissionFormat(serde.FormatJSON, permFormat{})
}

// PermissionJSON is the JSON message for a permission.
type PermissionJSON struct {
	Expressions map[string]ExpressionJSON
}

// ExpressionJSON is the JSON message for an expression.
type ExpressionJSON struct {
	Identities []json.RawMessage
	Matches    [][]int
}

// PermFormat is the format to encode and decode permission messages.
//
// - implements serde.FormatEngine
type permFormat struct{}

// Encode implements serde.FormatEngine. It encodes the permission message if
// appropriate, otherwise it returns an error.
func (permFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	perm, ok := msg.(*types.DisjunctivePermission)
	if !ok {
		return nil, xerrors.Errorf("invalid permission '%T'", msg)
	}

	expressions := make(map[string]ExpressionJSON)

	for key, expr := range perm.GetRules() {
		m, err := encodeExpression(ctx, expr)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode expression: %v", err)
		}

		expressions[key] = m
	}

	m := PermissionJSON{Expressions: expressions}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

func encodeExpression(ctx serde.Context, expr *types.Expression) (ExpressionJSON, error) {

	var m ExpressionJSON
	identities := make(types.IdentitySet, 0)

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
			return m, xerrors.Errorf("failed to serialize identity: %v", err)
		}

		identitiesRaw[i] = data
	}

	m.Identities = identitiesRaw
	m.Matches = matches

	return m, nil
}

// Decode implements serde.FormatEngine. It populates the permission from the
// data if appropriate, otherwise it returns an error.
func (permFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := PermissionJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	opts := make([]types.PermissionOption, 0, len(m.Expressions))

	for rule, raw := range m.Expressions {
		expr, err := decodeExpr(ctx, raw)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode expression: %v", err)
		}

		opts = append(opts, types.WithExpression(rule, expr))
	}

	return types.NewPermission(opts...), nil
}

func decodeExpr(ctx serde.Context, m ExpressionJSON) (*types.Expression, error) {
	fac := ctx.GetFactory(types.PublicKeyFac{})

	factory, ok := fac.(common.PublicKeyFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid public key factory '%T'", fac)
	}

	identities := make([]access.Identity, len(m.Identities))

	for i, raw := range m.Identities {
		pubkey, err := factory.PublicKeyOf(ctx, raw)
		if err != nil {
			return nil, xerrors.Errorf("public key: %v", err)
		}

		identities[i] = pubkey
	}

	matches := make([]types.IdentitySet, len(m.Matches))

	for i, indices := range m.Matches {
		matches[i] = make(types.IdentitySet, len(indices))

		for j, index := range indices {
			matches[i][j] = identities[index]
		}
	}

	return types.NewExpression(matches...), nil
}
