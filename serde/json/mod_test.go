package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestJSONEngine_GetFormat(t *testing.T) {
	ctx := NewContext()
	require.Equal(t, serde.FormatJSON, ctx.GetFormat())
}

func TestJSONEngine_Marshal(t *testing.T) {
	ctx := NewContext()

	data, err := ctx.Marshal(struct{}{})
	require.NoError(t, err)
	require.Equal(t, `{}`, string(data))

	_, err = ctx.Marshal(badObject{})
	require.EqualError(t, err, fake.Err("json: error calling MarshalJSON for type json.badObject"))
}

func TestJSONEngine_Unmarshal(t *testing.T) {
	ctx := NewContext()

	var m interface{}
	err := ctx.Unmarshal([]byte(`{"A":"B"}`), &m)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"A": "B"}, m)

	err = ctx.Unmarshal(nil, &m)
	require.EqualError(t, err, "unexpected end of JSON input")
}

// -----------------------------------------------------------------------------
// Utility functions

type badObject struct{}

func (o badObject) MarshalJSON() ([]byte, error) {
	return nil, fake.GetError()
}
