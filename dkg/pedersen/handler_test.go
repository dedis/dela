package pedersen

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func TestHandler_Stream_Deadline(t *testing.T) {
	old := recvTimeout
	defer func() {
		recvTimeout = old
	}()

	recvTimeout = time.Millisecond * 200

	out := &bytes.Buffer{}
	log := zerolog.New(out)

	h := Handler{
		dkgInstance: fakeHandler{running: false},
		log:         log,
	}

	err := h.Stream(nil, fake.NewBlockingReceiver())
	require.NoError(t, err)

	require.Regexp(t, "stream done, deadline exceeded", out.String())
}

func TestHandler_Stream_EOF(t *testing.T) {
	out := &bytes.Buffer{}
	log := zerolog.New(out)

	h := Handler{
		log: log,
	}

	err := h.Stream(nil, &eofReceiver{})
	require.NoError(t, err)

	require.Regexp(t, "stream done, EOF", out.String())
}

func TestHandler_StreamWrongMsg(t *testing.T) {
	h := Handler{
		dkgInstance: fakeHandler{err: fake.GetError()},
	}

	msg := fake.NewRecvMsg(fake.NewAddress(0), fake.Message{})

	err := h.Stream(nil, fake.NewReceiver(msg))
	require.EqualError(t, err, fake.Err("failed to handle message"))
}

func TestHandler_Stream(t *testing.T) {
	h := Handler{}

	err := h.Stream(fake.NewBadSender(), fake.NewBadReceiver())
	require.EqualError(t, err, fake.Err("failed to receive"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeHandler struct {
	dkgInstance
	running bool
	err     error
}

func (f fakeHandler) isRunning() bool {
	return f.running
}

func (f fakeHandler) handleMessage(ctx context.Context, msg serde.Message,
	from mino.Address, out mino.Sender) error {

	return f.err
}

type eofReceiver struct {
	mino.Receiver
}

func (r *eofReceiver) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
	return nil, nil, io.EOF
}
