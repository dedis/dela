package xml

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
)

func TestXMLEngine_GetFormat(t *testing.T) {
	ctx := NewContext()

	require.Equal(t, serde.FormatXML, ctx.GetFormat())
}

func TestXMLEngine_Marshal(t *testing.T) {
	ctx := NewContext()

	data, err := ctx.Marshal(testMessage{Value: 42})
	require.NoError(t, err)
	require.Equal(t, "<testMessage><Value>42</Value></testMessage>", string(data))
}

func TestXMLEngine_Unmarshal(t *testing.T) {
	ctx := NewContext()

	var m testMessage

	err := ctx.Unmarshal([]byte("<testMessage><Value>42</Value></testMessage>"), &m)
	require.NoError(t, err)
	require.Equal(t, 42, m.Value)
}

// -----------------------------------------------------------------------------
// Utility functions

type testMessage struct {
	Value int
}
