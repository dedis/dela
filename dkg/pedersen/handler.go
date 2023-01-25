package pedersen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

// the receiving time out, after which we check if the DKG setup is done or not.
// Allows to exit the loop.
var recvTimeout = time.Second * 30

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	sync.RWMutex
	log zerolog.Logger

	dkgInstance dkgInstance
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address) *Handler {
	log := dela.Logger.With().Str("role", "DKG handler").Str("addr", me.String()).Logger()

	return &Handler{
		log: log,

		dkgInstance: newInstance(log, me, privKey),
	}
}

// Stream implements mino.Handler. It allows one to stream messages to the
// players.
func (h *Handler) Stream(out mino.Sender, in mino.Receiver) error {
	// Note: one should never assume any synchronous properties on the messages.
	// For example we can not expect to receive the start message from the
	// initiator of the DKG protocol first because some node could have received
	// this start message earlier than us, start their DKG work by sending
	// messages to the other nodes, and then we might get their messages before
	// the start message.

	h.log.Info().Msg("stream start")

	globalCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), recvTimeout)
		from, msg, err := in.Recv(ctx)
		cancel()

		if errors.Is(err, context.DeadlineExceeded) {
			if !h.dkgInstance.isRunning() {
				h.log.Info().Msg("stream done, deadline exceeded")
				return nil
			}
			continue
		}

		if errors.Is(err, io.EOF) {
			h.log.Info().Msg("stream done, EOF")
			return nil
		}

		if err != nil {
			return xerrors.Errorf("failed to receive: %v", err)
		}

		h.log.Info().Str("from", from.String()).Str("type", fmt.Sprintf("%T", msg)).
			Msg("message received")

		err = h.dkgInstance.handleMessage(globalCtx, msg, from, out)
		if err != nil {
			return xerrors.Errorf("failed to handle message: %v", err)
		}
	}
}
