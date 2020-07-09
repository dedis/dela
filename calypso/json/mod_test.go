package json

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/calypso"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

func TestRecordFormat_New(t *testing.T) {
	rf := newRecordFormat()
	require.IsType(t, recordFormat{}, rf)
}

func TestRecordFormat_Encode(t *testing.T) {
	rf := newRecordFormat()
	ctx := fake.NewContext()
	record := calypso.NewRecord(rf.suite.Point(), rf.suite.Point(), fake.NewAccessControl())

	data, err := rf.Encode(ctx, record)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("{\"K\":\"%s\",\"C\":\"%s\",\"AC\":{}}", testPoint, testPoint), string(data))

	ctx = fake.NewBadContext()
	_, err = rf.Encode(ctx, record)
	require.EqualError(t, err, "couldn't marshal: fake error")

	ctx = fake.NewContext()

	record = calypso.NewRecord(rf.suite.Point(), badpoint{}, fake.NewAccessControl())
	_, err = rf.Encode(ctx, record)
	require.EqualError(t, err, "failed to marshal C: oops")

	record = calypso.NewRecord(badpoint{}, rf.suite.Point(), fake.NewAccessControl())
	_, err = rf.Encode(ctx, record)
	require.EqualError(t, err, "failed to marshal K: oops")

	record = calypso.NewRecord(rf.suite.Point(), rf.suite.Point(), fake.NewBadAccessControl())
	_, err = rf.Encode(ctx, record)
	require.EqualError(t, err, "failed to serialize the access control: fake error")

	_, err = rf.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestRecordFormat_Decode(t *testing.T) {
	rf := newRecordFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, calypso.AccessKeyFac{}, fake.NewAccessControlFactory())

	data := []byte(fmt.Sprintf(`{"K":"%s","C":"%s","AC":{}}`, testPoint, testPoint))
	msg, err := rf.Decode(ctx, data)
	require.NoError(t, err)
	require.IsType(t, calypso.Record{}, msg)

	data = []byte(fmt.Sprintf(`{"K":"%s","C":"","AC":{}}`, testPoint))
	_, err = rf.Decode(ctx, data)
	require.EqualError(t, err, "failed to unmarshal C: invalid Ed25519 curve point")

	data = []byte(fmt.Sprintf(`{"K":"","C":"%s","AC":{}}`, testPoint))
	_, err = rf.Decode(ctx, data)
	require.EqualError(t, err, "failed to unmarshal K: invalid Ed25519 curve point")

	ctx = serde.WithFactory(ctx, calypso.AccessKeyFac{}, fake.NewBadAccessControlFactory())
	data = []byte(fmt.Sprintf(`{"K":"%s","C":"%s","AC":{}}`, testPoint, testPoint))
	_, err = rf.Decode(ctx, data)
	require.EqualError(t, err, "failed to deserialize the access control: fake error")

	ctx = serde.WithFactory(ctx, calypso.AccessKeyFac{}, nil)
	data = []byte(fmt.Sprintf(`{"K":"%s","C":"%s","AC":{}}`, testPoint, testPoint))
	_, err = rf.Decode(ctx, data)
	require.EqualError(t, err, "invalid factory of type '<nil>'")

	data = []byte(fmt.Sprintf(`{"K":"%s","C":"%s","AC":{}}`, testPoint, testPoint))
	_, err = rf.Decode(fake.NewBadContext(), data)
	require.EqualError(t, err, "couldn't unmarshal record: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

const testPoint = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type badpoint struct {
	kyber.Point
}

func (p badpoint) MarshalBinary() ([]byte, error) {
	return nil, xerrors.New("oops")
}
