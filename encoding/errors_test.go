package encoding

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestEncodingError(t *testing.T) {
	e := xerrors.New("oops")
	err := NewEncodingError("abc", e)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, NewEncodingError("abc", nil)))
	require.False(t, xerrors.Is(err, NewEncodingError("other", nil)))
	require.EqualError(t, err, "couldn't encode abc: oops")
}

func TestDecodingError(t *testing.T) {
	e := xerrors.New("oops")
	err := NewDecodingError("abc", e)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, NewDecodingError("abc", nil)))
	require.False(t, xerrors.Is(err, NewDecodingError("other", nil)))
	require.EqualError(t, err, "couldn't decode abc: oops")
}

func TestAnyEncodingError(t *testing.T) {
	e := xerrors.New("oops")
	err := NewAnyEncodingError(&empty.Empty{}, e)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, NewAnyEncodingError((*empty.Empty)(nil), nil)))
	require.False(t, xerrors.Is(err, NewAnyEncodingError((proto.Message)(nil), nil)))
	require.EqualError(t, err, "couldn't encode any *empty.Empty: oops")
}

func TestAnyDecodingError(t *testing.T) {
	e := xerrors.New("oops")
	err := NewAnyDecodingError(&empty.Empty{}, e)
	require.True(t, xerrors.Is(err, NewAnyDecodingError((*empty.Empty)(nil), nil)))
	require.False(t, xerrors.Is(err, NewAnyDecodingError((proto.Message)(nil), nil)))
	require.EqualError(t, err, "couldn't decode any *empty.Empty: oops")
}
