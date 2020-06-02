package encoder

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
)

func init() {
	serde.Register(TestMessage{})
	serde.Register(TestInnerMessage{})
}

type TestInnerMessage struct {
	Value string
}

type TestMessage struct {
	Value    string
	Specific TestInnerMessage
	Generic  serde.Raw
}

func TestJsonEncoder_WrapUnwrap(t *testing.T) {
	var encoder serde.Encoder = jsonEncoder{}

	inner, err := encoder.Wrap(TestInnerMessage{Value: "B"})
	require.NoError(t, err)

	msg := TestMessage{
		Value:    "A",
		Specific: TestInnerMessage{Value: "C"},
		Generic:  inner,
	}

	buffer, err := encoder.Wrap(msg)
	require.NoError(t, err)

	retItf, err := encoder.Unwrap(buffer)
	require.NoError(t, err)
	ret, ok := retItf.(*TestMessage)
	require.True(t, ok)
	require.Equal(t, "A", ret.Value)
	require.Equal(t, TestInnerMessage{Value: "C"}, ret.Specific)

	retInnerItf, err := encoder.Unwrap(ret.Generic)
	require.NoError(t, err)
	retInner, ok := retInnerItf.(*TestInnerMessage)
	require.True(t, ok)
	require.Equal(t, TestInnerMessage{Value: "B"}, *retInner)
}
