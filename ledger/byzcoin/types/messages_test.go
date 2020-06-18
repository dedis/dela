package types

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestGenesisPayload_Fingerprint(t *testing.T) {
	payload := GenesisPayload{root: []byte{5}}

	out := new(bytes.Buffer)
	err := payload.Fingerprint(out)
	require.NoError(t, err)
	require.Equal(t, "\x05", out.String())

	err = payload.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write root: fake error")
}

func TestBlockPayload_Fingerprint(t *testing.T) {
	payload := BlockPayload{
		root: []byte{6},
	}

	out := new(bytes.Buffer)
	err := payload.Fingerprint(out)
	require.NoError(t, err)
	require.Equal(t, "\x06", out.String())

	err = payload.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write root: fake error")
}
