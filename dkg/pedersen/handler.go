package pedersen

import (
	"context"
	"github.com/dlsniper/debugger"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
	"golang.org/x/xerrors"
)

// recvResponseTimeout is the maximum time a node will wait for a response
const recvResponseTimeout = time.Second * 10

// state is a struct contained in a handler that allows an actor to read the
// state of that handler. The actor should only use the getter functions to read
// the attributes.
type state struct {
	sync.Mutex
	distrKey     kyber.Point
	participants []mino.Address
}

func (s *state) Done() bool {
	s.Lock()
	defer s.Unlock()
	return s.distrKey != nil && s.participants != nil
}

func (s *state) GetDistKey() kyber.Point {
	s.Lock()
	defer s.Unlock()
	return s.distrKey
}

func (s *state) SetDistKey(key kyber.Point) {
	s.Lock()
	s.distrKey = key
	s.Unlock()
}

func (s *state) GetParticipants() []mino.Address {
	s.Lock()
	defer s.Unlock()
	return s.participants
}

func (s *state) SetParticipants(addrs []mino.Address) {
	s.Lock()
	s.participants = addrs
	s.Unlock()
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
	startRes  *state
	deals     chan dealFrom
	responses chan responseFrom
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address) *Handler {
	return &Handler{
		privKey:   privKey,
		me:        me,
		startRes:  &state{},
		deals:     make(chan dealFrom, 10000),
		responses: make(chan responseFrom, 10000),
	}
}

type dealFrom struct {
	deal types.Deal
	from mino.Address
}

type responseFrom struct {
	response *pedersen.Response
	from     mino.Address
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

	defer func() {
		dela.Logger.Trace().Msgf("We left stream ! BOOOHOOOO")
	}()
	dela.Logger.Warn().Msgf("We are streaming DKG ! WOOHOOOO")

	started := false

	// TODO: Different state machine if already initialized or not
	// if not initialized -> Deal/Response/Start
	// else -> Decrypt/Reshare/Deal/Response
	for {
		// TODO: Extract context, set timeout
		from, msg, err := in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive: %v", err)
		}

		dela.Logger.Trace().Msgf("%v received message from %v\n", h.me, from)

		// We expect a Start message or a decrypt request at first, but we might
		// receive other messages in the meantime, like a Deal.
		switch msg := msg.(type) {

		case types.Start:
			dela.Logger.Trace().Msgf("%v received start from %v\n", h.me, from)
			if started {
				dela.Logger.Warn().Msgf(
					"%v ignored start request from %v as it is already"+
						" started\n", h.me, from,
				)
				break
			}

			started = true
			errCh := make(chan error)
			ctx := context.Background()
			ctx, _ = context.WithTimeout(ctx, 5*time.Minute)
			go func() {

				debugger.SetLabels(
					func() []string {
						return []string{
							"module", "dkg",
							"operation", "start",
						}
					},
				)

				err := h.start(msg, from, out, ctx)
				if err != nil {
					errCh <- xerrors.Errorf("failed to start: %v", err)
				}
				close(errCh)
			}()

		case types.Deal:
			h.deals <- dealFrom{
				msg,
				from,
			}
			dela.Logger.Trace().Msgf("%v received deal from %v\n", h.me, from)

		case types.Response:
			response := &pedersen.Response{
				Index: msg.GetIndex(),
				Response: &vss.Response{
					SessionID: msg.GetResponse().GetSessionID(),
					Index:     msg.GetResponse().GetIndex(),
					Status:    msg.GetResponse().GetStatus(),
					Signature: msg.GetResponse().GetSignature(),
				},
			}

			h.responses <- responseFrom{
				response,
				from,
			}

			dela.Logger.Trace().Msgf(
				"%v received response from %v\n", h.me, from,
			)

		case types.DecryptRequest:
			dela.Logger.Trace().Msgf(
				"%v received decrypt request from %v\n", h.me, from,
			)
			if !h.startRes.Done() {
				return xerrors.Errorf(
					"you must first initialize DKG. Did you " +
						"call setup() first?",
				)
			}

			// TODO: check if started before
			h.RLock()
			S := suite.Point().Mul(h.privShare.V, msg.K)
			h.RUnlock()

			partial := suite.Point().Sub(msg.C, S)

			h.RLock()
			decryptReply := types.NewDecryptReply(
				// TODO: check if using the private index is the same as the public
				// index.
				int64(h.privShare.I),
				partial,
			)
			h.RUnlock()

			errs := out.Send(decryptReply, from)
			err = <-errs
			if err != nil {
				return xerrors.Errorf(
					"got an error while sending the decrypt "+
						"reply: %v", err,
				)
			}

		default:
			dela.Logger.Error().Msgf(
				"%v received an unsupported message %v from %v\n", h.me,
				msg, from,
			)
			return xerrors.Errorf(
				"expected Start message, decrypt request or "+
					"Deal as first message, got: %T", msg,
			)
		}
	}

	return nil
}

// start is called when the node has received its start message. Note that we
// might have already received some deals from other nodes in the meantime. The
// function handles the DKG creation protocol.
func (h *Handler) start(
	start types.Start, from mino.Address,
	out mino.Sender, ctx context.Context,
) error {

	dela.Logger.Info().Msgf("%v is starting a DKG", h.me)

	if len(start.GetAddresses()) != len(start.GetPublicKeys()) {
		return xerrors.Errorf(
			"there should be as many players as "+
				"pubKey: %d := %d", len(start.GetAddresses()),
			len(start.GetPublicKeys()),
		)
	}

	// 1. Create the DKG
	d, err := pedersen.NewDistKeyGenerator(
		suite, h.privKey, start.GetPublicKeys(), start.GetThreshold(),
	)
	if err != nil {
		return xerrors.Errorf("failed to create new DKG: %v", err)
	}
	h.dkg = d

	// 2. Send my Deals to the other nodes
	dela.Logger.Trace().Msgf("%v is generating its deals", h.me)
	deals, err := d.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

	// use a waitgroup to send all the deals asynchronously and wait
	var wg sync.WaitGroup
	wg.Add(len(deals))

	dela.Logger.Trace().Msgf("%s is sending its deals", h.me)
	for i, deal := range deals {
		dealMsg := types.NewDeal(
			deal.Index,
			deal.Signature,
			types.NewEncryptedDeal(
				deal.Deal.DHKey,
				deal.Deal.Signature,
				deal.Deal.Nonce,
				deal.Deal.Cipher,
			),
		)

		dela.Logger.Trace().Msgf("%s sent deal %d", h.me, i)
		errs := out.Send(dealMsg, start.GetAddresses()[i])
		go func(errs <-chan error) {
			err, more := <-errs
			if more {
				dela.Logger.Warn().Msgf(
					"got an error while sending deal: %v", err,
				)
			}
			wg.Done()
		}(errs)
	}

	wg.Wait()

	dela.Logger.Debug().Msgf("%s sent all its deals", h.me)

	// Process the deals we received before the start message
	dela.Logger.Trace().Msgf("%v is handling deals from other nodes", h.me)

	numReceivedDeals := 0
	for numReceivedDeals < len(deals) {
		select {
		case df := <-h.deals:
			err = h.handleDeal(df.deal, df.from, start.GetAddresses(), out)
			if err != nil {
				dela.Logger.Warn().Msgf(
					"%s failed to handle received deal "+
						"from %s: %v", h.me, from, err,
				)
			} else {
				dela.Logger.Trace().Msgf(
					"%s handled deal #%v from %s",
					h.me, numReceivedDeals, df.from,
				)
				numReceivedDeals++
			}

		case <-ctx.Done():
			dela.Logger.Error().Msgf("%s timed out while receiving deals", h.me)
			return xerrors.Errorf("timed out while receiving deals")
		}
	}

	dela.Logger.Debug().Msgf("%v received all the expected deals", h.me)

	h.startRes.SetParticipants(start.GetAddresses())

	dela.Logger.Trace().Msgf("%v is certifying dkg", h.me)
	err = h.certify(ctx)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	dela.Logger.Trace().Msgf("%s is certified", h.me)

	err = h.announceDkgPublicKey(out, from)
	if err != nil {
		return xerrors.Errorf("failed to announce dkg public key: %v", err)
	}

	dela.Logger.Info().Msgf("%s announced the DKG public key", h.me)

	return nil
}

func (h *Handler) certify(ctx context.Context) error {
	for !h.dkg.Certified() {
		select {
		case rf, ok := <-h.responses:
			if !ok {
				dela.Logger.Error().Msgf("Aborting certification - channel has been closed")
				return xerrors.Errorf("certification aborted: channel closed")
			}

			dela.Logger.Trace().Msgf(
				"%s about to handle response from %s",
				h.me, rf.from,
			)
			_, err := h.dkg.ProcessResponse(rf.response)
			if err != nil {
				dela.Logger.Warn().Msgf(
					"%s failed to process response: %v", h.me, err,
				)
			} else {
				dela.Logger.Trace().Msgf(
					"%s handled response from %s",
					h.me, rf.from,
				)
			}

		case <-ctx.Done():
			dela.Logger.Error().Msgf(
				"%s timed out while receiving responses", h.me,
			)
			return xerrors.Errorf("timed out while receiving responses")
		}
	}

	dela.Logger.Trace().Msgf("%s left certification phase", h.me)

	return nil
}

func (h *Handler) announceDkgPublicKey(
	out mino.Sender, from mino.Address,
) error {
	// 6. Send back the public DKG key
	distrKey, err := h.dkg.DistKeyShare()
	if err != nil {
		return xerrors.Errorf("failed to get distr key: %v", err)
	}

	// 7. Update the state before sending to acknowledgement to the
	// orchestrator, so that it can process decrypt requests right away.
	h.startRes.SetDistKey(distrKey.Public())

	h.Lock()
	h.privShare = distrKey.PriShare()
	h.Unlock()

	done := types.NewStartDone(distrKey.Public())
	err = <-out.Send(done, from)
	if err != nil {
		return xerrors.Errorf("got an error while sending pub key: %v", err)
	}

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (h *Handler) handleDeal(
	msg types.Deal, from mino.Address, addrs []mino.Address,
	out mino.Sender,
) error {

	dela.Logger.Trace().Msgf("%s received deal from %s", h.me, from)

	deal := &pedersen.Deal{
		Index: msg.GetIndex(),
		Deal: &vss.EncryptedDeal{
			DHKey:     msg.GetEncryptedDeal().GetDHKey(),
			Signature: msg.GetEncryptedDeal().GetSignature(),
			Nonce:     msg.GetEncryptedDeal().GetNonce(),
			Cipher:    msg.GetEncryptedDeal().GetCipher(),
		},
		Signature: msg.GetSignature(),
	}

	dela.Logger.Trace().Msgf("%v processing deal from %v\n", h.me, from)
	response, err := h.dkg.ProcessDeal(deal)
	if err != nil {
		return xerrors.Errorf(
			"failed to process deal from %s: %v",
			h.me, err,
		)
	}

	resp := types.NewResponse(
		response.Index,
		types.NewDealerResponse(
			response.Response.Index,
			response.Response.Status,
			response.Response.SessionID,
			response.Response.Signature,
		),
	)

	for _, addr := range addrs {
		if addr.Equal(h.me) {
			continue
		}

		dela.Logger.Trace().Msgf("%v sending response to %v\n", h.me, addr)
		errs := out.Send(resp, addr)
		err = <-errs
		if err != nil {
			dela.Logger.Warn().Msgf(
				"got an error while sending "+
					"response: %v", err,
			)
			return xerrors.Errorf(
				"failed to send response to '%s': %v", addr, err,
			)
		}

	}

	return nil
}
