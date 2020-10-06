package threshold

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestThresholdHandler_Stream(t *testing.T) {
	handler := newHandler(
		&Threshold{signer: fake.NewAggregateSigner()},
		fakeReactor{},
	)

	recv := fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), cosi.SignatureRequest{Value: fake.Message{}}),
	)
	sender := fake.NewBadSender()

	err := handler.Stream(sender, recv)
	require.NoError(t, err)
}

func TestThresholdHandler_BadStream_Stream(t *testing.T) {
	handler := newHandler(
		&Threshold{},
		fakeReactor{},
	)

	recv := fake.NewBadReceiver()

	err := handler.Stream(fake.Sender{}, recv)
	require.EqualError(t, err, fake.Err("failed to receive"))
}

func TestThresholdHandler_UnsupportedMessage_Stream(t *testing.T) {
	logger, check := fake.CheckLog("invalid request type 'fake.Message'")

	handler := newHandler(
		&Threshold{
			logger: logger,
		},
		fakeReactor{},
	)

	recv := fake.NewReceiver(fake.NewRecvMsg(fake.NewAddress(0), fake.Message{}))

	err := handler.Stream(fake.Sender{}, recv)
	require.NoError(t, err)
	check(t)
}

func TestThresholdHandler_BadReactor_Stream(t *testing.T) {
	logger, check := fake.CheckLog(fake.Err("couldn't hash message"))

	handler := newHandler(
		&Threshold{
			logger: logger,
		},
		fakeReactor{err: fake.GetError()},
	)

	recv := fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), cosi.SignatureRequest{Value: fake.Message{}}),
	)

	err := handler.Stream(fake.Sender{}, recv)
	require.NoError(t, err)
	check(t)
}

func TestThresholdHandler_BadSigner_Stream(t *testing.T) {
	logger, check := fake.CheckLog(fake.Err("couldn't sign"))

	handler := newHandler(
		&Threshold{
			logger: logger,
		},
		fakeReactor{},
	)

	handler.signer = fake.NewBadSigner()
	recv := fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), cosi.SignatureRequest{Value: fake.Message{}}),
	)

	err := handler.Stream(fake.Sender{}, recv)
	require.NoError(t, err)
	check(t)
}
