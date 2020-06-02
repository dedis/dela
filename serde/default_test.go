package serde

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnimplemented_Message(t *testing.T) {
	var m Message = UnimplementedMessage{}

	_, err := m.VisitGob()
	require.EqualError(t, err, notImplementedErr)
	_, err = m.VisitJSON()
	require.EqualError(t, err, notImplementedErr)
	_, err = m.VisitProto()
	require.EqualError(t, err, notImplementedErr)
}

func TestUnimplemented_Factory(t *testing.T) {
	var f Factory = UnimplementedFactory{}

	_, err := f.VisitGob(nil)
	require.EqualError(t, err, notImplementedErr)
	_, err = f.VisitJSON(nil)
	require.EqualError(t, err, notImplementedErr)
	_, err = f.VisitProto(nil)
	require.EqualError(t, err, notImplementedErr)
}
