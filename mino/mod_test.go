package mino

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsupportedHandler_Process(t *testing.T) {
	h := UnsupportedHandler{}
	resp, err := h.Process(Request{})
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestUnsupportedHandler_Stream(t *testing.T) {
	h := UnsupportedHandler{}
	require.Error(t, h.Stream(nil, nil))
}
