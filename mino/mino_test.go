package mino

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
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

func TestMustCreateRPC(t *testing.T) {
	rpc := MustCreateRPC(fakeMino{}, "name", nil, nil)
	require.NotNil(t, rpc)
}

func TestMustCreateRPC_Panic(t *testing.T) {
	defer func() {
		err := recover().(error)
		require.EqualError(t, err, "rpc_error")
	}()

	MustCreateRPC(fakeMino{err: xerrors.New("rpc_error")}, "name", nil, nil)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeRPC struct {
	RPC
}

type fakeMino struct {
	Mino

	err error
}

func (m fakeMino) CreateRPC(name string, h Handler, f serde.Factory) (RPC, error) {
	return fakeRPC{}, m.err
}
