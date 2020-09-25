package threshold

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestThresholdHandler_Stream(t *testing.T) {
	handler := newHandler(
		&CoSi{signer: fake.NewAggregateSigner()},
		fakeReactor{},
	)

	rcvr := &fakeReceiver{resps: makeResponse()}
	sender := fakeSender{}

	err := handler.Stream(sender, rcvr)
	require.NoError(t, err)

	rcvr.err = xerrors.New("oops")
	err = handler.processRequest(sender, rcvr)
	require.EqualError(t, err, "failed to receive: oops")

	err = handler.processRequest(sender, &fakeReceiver{resps: makeBadResponse()})
	require.EqualError(t, err, "invalid request type 'fake.Message'")

	handler.reactor = fakeReactor{err: xerrors.New("oops")}
	rcvr.err = nil
	rcvr.resps = makeResponse()
	err = handler.processRequest(sender, rcvr)
	require.EqualError(t, err, "couldn't hash message: oops")

	handler.reactor = fakeReactor{}
	handler.signer = fake.NewBadSigner()
	rcvr.resps = makeResponse()
	err = handler.processRequest(sender, rcvr)
	require.EqualError(t, err, fake.Err("couldn't sign"))

	handler.signer = fake.NewAggregateSigner()
	sender = fakeSender{numErr: 1}
	rcvr.resps = makeResponse()
	err = handler.Stream(sender, rcvr)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeResponse() [][]interface{} {
	return [][]interface{}{{fake.Address{}, cosi.SignatureRequest{Value: fake.Message{}}}}
}

func makeBadResponse() [][]interface{} {
	return [][]interface{}{{fake.Address{}, fake.Message{}}}
}
