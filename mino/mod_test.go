package mino

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
)

func TestUnsupportedHandler_Process(t *testing.T) {
	h := UnsupportedHandler{}
	resp, err := h.Process(nil)
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestUnsupportedHandler_Combine(t *testing.T) {
	h := UnsupportedHandler{}

	messages := []proto.Message{&empty.Empty{}, &empty.Empty{}}
	resps, err := h.Combine(messages)
	require.NoError(t, err)
	require.Equal(t, messages, resps)
}

func TestUnsupportedHandler_Stream(t *testing.T) {
	h := UnsupportedHandler{}
	require.Error(t, h.Stream(nil, nil))
}
