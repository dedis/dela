package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serdeng"
)

func TestFormat_Decode(t *testing.T) {
	format := format{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	pubkey, err := format.Decode(ctx, []byte(`{"Name": "fake","Data":[]}`))
	require.NoError(t, err)
	require.Equal(t, common.NewAlgorithm("fake"), pubkey)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{"Name": "unknown"}`))
	require.EqualError(t, err, "couldn't deserialize algorithm: fake error")
}
