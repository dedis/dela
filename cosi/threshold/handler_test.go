package threshold

import (
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestThresholdHandler_Stream(t *testing.T) {
	handler := newHandler(
		&CoSi{signer: fake.NewSigner(), encoder: encoding.NewProtoEncoder()},
		fakeHashable{},
	)

	rcvr := &fakeReceiver{resps: makeResponse()}
	sender := fakeSender{}

	err := handler.Stream(sender, rcvr)
	require.NoError(t, err)

	handler.hasher = fakeHashable{err: xerrors.New("oops")}
	rcvr.resps = makeResponse()
	err = handler.processRequest(sender, rcvr)
	require.EqualError(t, err, "couldn't hash message: oops")

	handler.hasher = fakeHashable{}
	handler.signer = fake.NewBadSigner()
	rcvr.resps = makeResponse()
	err = handler.processRequest(sender, rcvr)
	require.EqualError(t, err, "couldn't sign: fake error")

	handler.signer = fake.NewSigner()
	handler.encoder = fake.BadPackEncoder{}
	rcvr.resps = makeResponse()
	err = handler.processRequest(sender, rcvr)
	require.EqualError(t, err, "couldn't pack signature: fake error")

	handler.encoder = encoding.NewProtoEncoder()
	sender = fakeSender{numErr: 1}
	rcvr.resps = makeResponse()
	err = handler.Stream(sender, rcvr)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeResponse() [][]interface{} {
	return [][]interface{}{{fake.Address{}, &empty.Empty{}}}
}
