package minoch

import (
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
)

func TestRPC_Call(t *testing.T) {
	manager := NewManager()

	m1, err := NewMinoch(manager, "A")
	require.NoError(t, err)
	rpc1, err := m1.MakeRPC("test", testHandler{})
	require.NoError(t, err)

	m2, err := NewMinoch(manager, "B")
	require.NoError(t, err)
	_, err = m2.MakeRPC("test", testHandler{})

	resps, errs := rpc1.Call(&empty.Empty{}, m2)
	select {
	case <-resps:
		t.Fatal("an error is expected")
	case <-errs:
	}
}
