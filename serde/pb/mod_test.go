package pb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
)

func init() {
	serde.Register(TestMessage{})
}

type TestMessage struct {
	Str string
}

func TestPbEncoder_Encode(t *testing.T) {
	encoder := newEncoder()

	buffer, err := encoder.Encode(TestMessage{Str: "Hello World!"})
	require.NoError(t, err)

	ret := &TestMessage{}
	err = encoder.Decode(buffer, ret)
	require.NoError(t, err)
	require.Equal(t, "Hello World!", ret.Str)
}

func TestPbEncoder_Wrap(t *testing.T) {
	encoder := newEncoder()

	buffer, err := encoder.Wrap(TestMessage{Str: "Hello World!"})
	require.NoError(t, err)

	retItf, err := encoder.Unwrap(buffer)
	require.NoError(t, err)

	ret, ok := retItf.(*TestMessage)
	require.True(t, ok)
	require.Equal(t, "Hello World!", ret.Str)
}
