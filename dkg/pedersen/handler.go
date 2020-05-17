package pedersen

import (
	"context"
	fmt "fmt"
	"sync"
	"time"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

// Suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

// recvResponseTimeout is the maximum time a node will wait for a response
const recvResponseTimeout = time.Second * 10

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	sync.RWMutex
	addressFactory mino.AddressFactory
	dkg            *pedersen.DistKeyGenerator
	privKey        kyber.Scalar
	me             mino.Address
	suite          suites.Suite
	privShare      *share.PriShare
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar,
	af mino.AddressFactory, me mino.Address, suite suites.Suite) *Handler {

	return &Handler{
		addressFactory: af,
		privKey:        privKey,
		me:             me,
		suite:          suite,
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

	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("failed to receive: %v", err)
	}

	deals := []*Deal{}

	// We expect a Start message or a decrypt request at first, but we might
	// receive other messages in the meantime, like a Deal.
mainSwitch:
	switch msg := msg.(type) {

	case *Start:
		err := h.start(msg, deals, from, out, in)
		if err != nil {
			return xerrors.Errorf("failed to start: %v", err)
		}

	case *Deal:
		// This is a special case where a DKG started, some nodes received the
		// start signal and started sending their deals but we have not yet
		// received our start signal. In this case we collect the Deals while
		// waiting for the start signal.
		deals = append(deals, msg)
		goto mainSwitch

	case *DecryptRequest:
		// TODO: check if started before
		K := h.suite.Point()
		err := K.UnmarshalBinary(msg.K)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal K: %v", err)
		}

		C := h.suite.Point()
		err = C.UnmarshalBinary(msg.C)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal C: %v", err)
		}

		h.RLock()
		S := suite.Point().Mul(h.privShare.V, K)
		h.RUnlock()

		partial := suite.Point().Sub(C, S)

		VBuf, err := partial.MarshalBinary()
		if err != nil {
			return xerrors.Errorf("failed to marshal the partial: %v", err)
		}

		h.RLock()
		decryptReply := &DecryptReply{
			V: VBuf,
			// TODO: check if using the private index is the same as the public
			// index.
			I: int64(h.privShare.I),
		}
		h.RUnlock()

		errs := out.Send(decryptReply, from)
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
func (h *Handler) start(start *Start, receivedDeals []*Deal, from mino.Address,
	out mino.Sender, in mino.Receiver) error {

	if len(start.Addresses) != len(start.PubKeys) {
		return xerrors.Errorf("there should be as many players as "+
			"pubKey: %d := %d", len(start.Addresses), len(start.PubKeys))
	}

	addrs := make([]mino.Address, len(start.Addresses))
	pubKeys := make([]kyber.Point, len(start.PubKeys))

	for i, addrBuf := range start.Addresses {
		addr := h.addressFactory.FromText(addrBuf)
		if addr == nil {
			return xerrors.Errorf("failed to unmarsahl address '%s'", addr)
		}

		pubkey := h.suite.Point()
		err := pubkey.UnmarshalBinary(start.PubKeys[i])
		if err != nil {
			return xerrors.Errorf("failed to unmarshal pubkey: %v", err)
		}

		addrs[i] = addr
		pubKeys[i] = pubkey
	}

	// 1. Create the DKG
	d, err := pedersen.NewDistKeyGenerator(suite, h.privKey, pubKeys, int(start.T))
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
		dealMsg := &Deal{
			Index: deal.Index,
			EncryptedDeal: &Deal_EncryptedDeal{
				Dhkey:     deal.Deal.DHKey,
				Signature: deal.Deal.Signature,
				Nonce:     deal.Deal.Nonce,
				Cipher:    deal.Deal.Cipher,
			},
			Signature: deal.Signature,
		}

		errs := out.Send(dealMsg, addrs[i])
		go func(errs <-chan error) {
			err, more := <-errs
			if more {
				fabric.Logger.Warn().Msgf("got an error while sending deal: %v", err)
			}
			wg.Done()
		}(errs)
	}

	wg.Wait()

	fabric.Logger.Trace().Msgf("%s sent all its deals", h.me)

	receivedResps := make([]*pedersen.Response, 0)

	numReceivedDeals := 0

	// Process the deals we received before the start message
	for _, deal := range receivedDeals {
		err = h.handleDeal(deal, from, addrs, out)
		if err != nil {
			fabric.Logger.Warn().Msgf("%s failed to handle received deal "+
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

		switch msg := msg.(type) {

		case *Deal:
			// 4. Process the Deal and Send the response to all the other nodes
			err = h.handleDeal(msg, from, addrs, out)
			if err != nil {
				fabric.Logger.Warn().Msgf("%s failed to handle received deal "+
					"from %s: %v", h.me, from, err)
			}
			numReceivedDeals++
			fmt.Printf("%v received %d/%d deals\n", h.me, numReceivedDeals, len(deals))

		case *Response:
			// 5. Processing responses
			fabric.Logger.Trace().Msgf("%s received response from %s", h.me, from)
			response := &pedersen.Response{
				Index: msg.Index,
				Response: &vss.Response{
					SessionID: msg.Response.SessionID,
					Index:     msg.Response.Index,
					Status:    msg.Response.Status,
					Signature: msg.Response.Signature,
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
			fabric.Logger.Warn().Msgf("%s failed to process response "+
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

		switch msg := msg.(type) {

		case *Response:
			// 5. Processing responses
			fabric.Logger.Trace().Msgf("%s received response from %s", h.me, from)
			response := &pedersen.Response{
				Index: msg.Index,
				Response: &vss.Response{
					SessionID: msg.Response.SessionID,
					Index:     msg.Response.Index,
					Status:    msg.Response.Status,
					Signature: msg.Response.Signature,
				},
			}

			_, err = h.dkg.ProcessResponse(response)
			if err != nil {
				fabric.Logger.Warn().Msgf("%s, failed to process response "+
					"from '%s': %v", h.me, from, err)
			}

		default:
			return xerrors.Errorf("expected a response, got: %T", msg)
		}
	}

	fabric.Logger.Trace().Msgf("%s is certified", h.me)

	// 6. Send back the public DKG key
	distrKey, err := h.dkg.DistKeyShare()
	if err != nil {
		return xerrors.Errorf("failed to get distr key: %v", err)
	}

	distrKeyBuf, err := distrKey.Public().MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal distr pub key: %v", err)
	}

	done := &StartDone{PubKey: distrKeyBuf}
	errs := out.Send(done, from)
	err = <-errs
	if err != nil {
		return xerrors.Errorf("got an error while sending pub key: %v", err)
	}

	h.Lock()
	h.privShare = distrKey.PriShare()
	h.Unlock()

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (h *Handler) handleDeal(msg *Deal, from mino.Address, addrs []mino.Address,
	out mino.Sender) error {

	fabric.Logger.Trace().Msgf("%s received deal from %s", h.me, from)

	deal := &pedersen.Deal{
		Index: msg.Index,
		Deal: &vss.EncryptedDeal{
			DHKey:     msg.EncryptedDeal.Dhkey,
			Signature: msg.EncryptedDeal.Signature,
			Nonce:     msg.EncryptedDeal.Nonce,
			Cipher:    msg.EncryptedDeal.Cipher,
		},
		Signature: msg.Signature,
	}

	response, err := h.dkg.ProcessDeal(deal)
	if err != nil {
		return xerrors.Errorf("failed to process deal from %s: %v",
			h.me, err)
	}

	respProto := &Response{
		Index: response.Index,
		Response: &Response_Data{
			SessionID: response.Response.SessionID,
			Index:     response.Response.Index,
			Status:    response.Response.Status,
			Signature: response.Response.Signature,
		},
	}

	for _, addr := range addrs {
		if addr.Equal(h.me) {
			continue
		}

		errs := out.Send(respProto, addr)
		err = <-errs
		if err != nil {
			fabric.Logger.Warn().Msgf("got an error while sending "+
				"response: %v", err)
		}

	}

	return nil
}
