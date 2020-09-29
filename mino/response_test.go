package mino

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestResponse_GetFrom(t *testing.T) {
	resp := NewResponse(fakeAddr{}, fakeMsg{})

	require.Equal(t, fakeAddr{}, resp.GetFrom())
}

func TestResponse_GetMessageOrError(t *testing.T) {
	resp := NewResponse(fakeAddr{}, fakeMsg{})
	msg, err := resp.GetMessageOrError()
	require.NoError(t, err)
	require.Equal(t, fakeMsg{}, msg)

	resp = NewResponseWithError(fakeAddr{}, xerrors.New("oops"))
	_, err = resp.GetMessageOrError()
	require.EqualError(t, err, "oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeMsg struct {
	serde.Message
}
