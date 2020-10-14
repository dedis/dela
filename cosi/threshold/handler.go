// This file contains the implementation of the RPC handler.
//
// Documentation Last Review: 05.10.2020
//

package threshold

import (
	"context"
	"io"

	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// thresholdHandler is an implementation of mino.Handler for a threshold
// collective signing.
type thresholdHandler struct {
	*Threshold
	mino.UnsupportedHandler

	reactor cosi.Reactor
}

func newHandler(c *Threshold, hasher cosi.Reactor) thresholdHandler {
	return thresholdHandler{
		Threshold: c,
		reactor:   hasher,
	}
}

// Stream implements mino.RPC. It listens for incoming messages and tries to
// send back the signature. If the message is malformed, it is ignored.
func (h thresholdHandler) Stream(out mino.Sender, in mino.Receiver) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		addr, msg, err := in.Recv(ctx)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("failed to receive: %v", err)
		}

		err = h.processRequest(out, msg, addr)
		if err != nil {
			h.logger.Warn().Err(err).Send()
		}
	}
}

func (h thresholdHandler) processRequest(sender mino.Sender, msg serde.Message, addr mino.Address) error {
	req, ok := msg.(cosi.SignatureRequest)
	if !ok {
		return xerrors.Errorf("invalid request type '%T'", msg)
	}

	buffer, err := h.reactor.Invoke(addr, req.Value)
	if err != nil {
		return xerrors.Errorf("couldn't hash message: %v", err)
	}

	signature, err := h.signer.Sign(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't sign: %v", err)
	}

	resp := cosi.SignatureResponse{
		Signature: signature,
	}

	err = <-sender.Send(resp, addr)
	if err != nil {
		return xerrors.Errorf("couldn't send the response: %v", err)
	}

	return nil
}
