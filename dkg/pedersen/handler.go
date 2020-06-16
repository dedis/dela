package pedersen

import (
	"context"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
	"golang.org/x/xerrors"
)

// recvResponseTimeout is the maximum time a node will wait for a response
const recvResponseTimeout = time.Second * 10

// startResult holds the result of the DKG initialization, which is needed by an
// actor to perform the Encrypt/Decrypt.
type startResult struct {
	distrKey kyber.Point
}

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	sync.RWMutex
	dkg       *pedersen.DistKeyGenerator
	privKey   kyber.Scalar
	me        mino.Address
	privShare *share.PriShare
	factory   serde.Factory
	startRes  *startResult
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address, f serde.Factory) *Handler {
	return &Handler{
		privKey:  privKey,
		me:       me,
		factory:  f,
		startRes: &startResult{},
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

	deals := []Deal{}

mainSwitch:
	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("failed to receive: %v", err)
	}

	req := tmp.FromProto(msg, h.factory)

	// We expect a Start message or a decrypt request at first, but we might
	// receive other messages in the meantime, like a Deal.
	switch msg := req.(type) {

	case Start:
		err := h.start(msg, deals, from, out, in)
		if err != nil {
			return xerrors.Errorf("failed to start: %v", err)
		}

	case Deal:
		// This is a special case where a DKG started, some nodes received the
		// start signal and started sending their deals but we have not yet
		// received our start signal. In this case we collect the Deals while
		// waiting for the start signal.
		deals = append(deals, msg)
		goto mainSwitch

	case DecryptRequest:
		// TODO: check if started before
		h.RLock()
		S := suite.Point().Mul(h.privShare.V, msg.K)
		h.RUnlock()

		partial := suite.Point().Sub(msg.C, S)

		h.RLock()
		decryptReply := DecryptReply{
			V: partial,
			// TODO: check if using the private index is the same as the public
			// index.
			I: int64(h.privShare.I),
		}
		h.RUnlock()

		errs := out.Send(tmp.ProtoOf(decryptReply), from)
		err = <-errs
		if err != nil {
			return xerrors.Errorf("got an error while sending the decrypt "+
				"reply: %v", err)
		}

	default:
		return xerrors.Errorf("expected Start message, decrypt request or "+
			"Deal as first message, got: %T", msg)
	}

	return nil
}

// start is called when the node has received its start message. Note that we
// might have already received some deals from other nodes in the meantime. The
// function handles the DKG creation protocol.
func (h *Handler) start(start Start, receivedDeals []Deal, from mino.Address,
	out mino.Sender, in mino.Receiver) error {

	if len(start.addresses) != len(start.pubkeys) {
		return xerrors.Errorf("there should be as many players as "+
			"pubKey: %d := %d", len(start.addresses), len(start.pubkeys))
	}

	// 1. Create the DKG
	d, err := pedersen.NewDistKeyGenerator(suite, h.privKey, start.pubkeys, start.t)
	if err != nil {
		return xerrors.Errorf("failed to create new DKG: %v", err)
	}
	h.dkg = d

	// 2. Send my Deals to the other nodes
	deals, err := d.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

	// use a waitgroup to send all the deals asynchronously and wait
	var wg sync.WaitGroup
	wg.Add(len(deals))

	for i, deal := range deals {
		dealMsg := Deal{
			index: deal.Index,
			encryptedDeal: EncryptedDeal{
				dhkey:     deal.Deal.DHKey,
				signature: deal.Deal.Signature,
				nonce:     deal.Deal.Nonce,
				cipher:    deal.Deal.Cipher,
			},
			signature: deal.Signature,
		}

		errs := out.Send(tmp.ProtoOf(dealMsg), start.addresses[i])
		go func(errs <-chan error) {
			err, more := <-errs
			if more {
				dela.Logger.Warn().Msgf("got an error while sending deal: %v", err)
			}
			wg.Done()
		}(errs)
	}

	wg.Wait()

	dela.Logger.Trace().Msgf("%s sent all its deals", h.me)

	receivedResps := make([]*pedersen.Response, 0)

	numReceivedDeals := 0

	// Process the deals we received before the start message
	for _, deal := range receivedDeals {
		err = h.handleDeal(deal, from, start.addresses, out)
		if err != nil {
			dela.Logger.Warn().Msgf("%s failed to handle received deal "+
				"from %s: %v", h.me, from, err)
		}
		numReceivedDeals++
	}

	// If there are N nodes, then N nodes first send (N-1) Deals. Then each node
	// send a response to every other nodes. So the number of responses a node
	// get is (N-1) * (N-1), where (N-1) should equal len(deals).
	for numReceivedDeals < len(deals) {
		from, msg, err := in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive after sending deals: %v", err)
		}

		resp := tmp.FromProto(msg, h.factory)

		switch msg := resp.(type) {

		case Deal:
			// 4. Process the Deal and Send the response to all the other nodes
			err = h.handleDeal(msg, from, start.addresses, out)
			if err != nil {
				dela.Logger.Warn().Msgf("%s failed to handle received deal "+
					"from %s: %v", h.me, from, err)
			}
			numReceivedDeals++

		case Response:
			// 5. Processing responses
			dela.Logger.Trace().Msgf("%s received response from %s", h.me, from)
			response := &pedersen.Response{
				Index: msg.index,
				Response: &vss.Response{
					SessionID: msg.response.sessionID,
					Index:     msg.response.index,
					Status:    msg.response.status,
					Signature: msg.response.signature,
				},
			}
			receivedResps = append(receivedResps, response)

		default:
			return xerrors.Errorf("undexpected message: %T", msg)
		}
	}

	for _, response := range receivedResps {
		_, err = h.dkg.ProcessResponse(response)
		if err != nil {
			dela.Logger.Warn().Msgf("%s failed to process response "+
				"from '%s': %v", h.me, from, err)
		}
	}

	for !h.dkg.Certified() {
		ctx, cancel := context.WithTimeout(context.Background(),
			recvResponseTimeout)
		defer cancel()

		from, msg, err := in.Recv(ctx)
		if err != nil {
			return xerrors.Errorf("failed to receive after sending deals: %v", err)
		}

		resp := tmp.FromProto(msg, h.factory)

		switch msg := resp.(type) {

		case Response:
			// 5. Processing responses
			dela.Logger.Trace().Msgf("%s received response from %s", h.me, from)
			response := &pedersen.Response{
				Index: msg.index,
				Response: &vss.Response{
					SessionID: msg.response.sessionID,
					Index:     msg.response.index,
					Status:    msg.response.status,
					Signature: msg.response.signature,
				},
			}

			_, err = h.dkg.ProcessResponse(response)
			if err != nil {
				dela.Logger.Warn().Msgf("%s, failed to process response "+
					"from '%s': %v", h.me, from, err)
			}

		default:
			return xerrors.Errorf("expected a response, got: %T", msg)
		}
	}

	dela.Logger.Trace().Msgf("%s is certified", h.me)

	// 6. Send back the public DKG key
	distrKey, err := h.dkg.DistKeyShare()
	if err != nil {
		return xerrors.Errorf("failed to get distr key: %v", err)
	}

	done := StartDone{pubkey: distrKey.Public()}
	errs := out.Send(tmp.ProtoOf(done), from)
	err = <-errs
	if err != nil {
		return xerrors.Errorf("got an error while sending pub key: %v", err)
	}

	h.Lock()
	h.privShare = distrKey.PriShare()
	h.startRes.distrKey = distrKey.Public()
	h.Unlock()

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (h *Handler) handleDeal(msg Deal, from mino.Address, addrs []mino.Address,
	out mino.Sender) error {

	dela.Logger.Trace().Msgf("%s received deal from %s", h.me, from)

	deal := &pedersen.Deal{
		Index: msg.index,
		Deal: &vss.EncryptedDeal{
			DHKey:     msg.encryptedDeal.dhkey,
			Signature: msg.encryptedDeal.signature,
			Nonce:     msg.encryptedDeal.nonce,
			Cipher:    msg.encryptedDeal.cipher,
		},
		Signature: msg.signature,
	}

	response, err := h.dkg.ProcessDeal(deal)
	if err != nil {
		return xerrors.Errorf("failed to process deal from %s: %v",
			h.me, err)
	}

	resp := Response{
		index: response.Index,
		response: DealerResponse{
			sessionID: response.Response.SessionID,
			index:     response.Response.Index,
			status:    response.Response.Status,
			signature: response.Response.Signature,
		},
	}

	for _, addr := range addrs {
		if addr.Equal(h.me) {
			continue
		}

		errs := out.Send(tmp.ProtoOf(resp), addr)
		err = <-errs
		if err != nil {
			dela.Logger.Warn().Msgf("got an error while sending "+
				"response: %v", err)
		}

	}

	return nil
}
