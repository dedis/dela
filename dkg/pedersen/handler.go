package pedersen

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
	"golang.org/x/xerrors"
)

// the receiving time out, after which we check if the DKG setup is done or not.
// Allows to exit the loop.
const recvTimeout = time.Second * 30

// the channel size used to stored buffered deals and responses. This arbitrary
// value must be set according to the characteristic of the system: number of
// nodes, networking, memory, etc...
// const chanSize = 10000

// constant used in the logs
const newState = "new state"

func newCryChan[T any](bufSize int) cryChan[T] {
	llvl := zerolog.NoLevel
	if os.Getenv("CRY_LVL") == "warn" {
		llvl = zerolog.WarnLevel
	}

	return cryChan[T]{
		c: make(chan T, bufSize),
		log: dela.Logger.With().Str("role", "cry chan").Int("size", bufSize).
			Logger().Level(llvl),
	}
}

type cryChan[T any] struct {
	c   chan T
	log zerolog.Logger
}

func (c *cryChan[T]) push(e T) {
	start := time.Now()
	select {
	case c.c <- e:
	case <-time.After(time.Second * 1):
		// prints the first 16 bytes of the trace, which should contain at least
		// the goroutine id.
		trace := make([]byte, 16)
		runtime.Stack(trace, false)
		c.log.Warn().Str("obj", fmt.Sprintf("%+v", e)).
			Str("trace", string(trace)).Msg("channel blocking")
		c.c <- e
		c.log.Warn().Str("obj", fmt.Sprintf("%+v", e)).
			Str("elapsed", time.Since(start).String()).
			Str("trace", string(trace)).Msg("channel unblocked")
	}
}

func (c *cryChan[T]) pop(ctx context.Context) (t T, err error) {
	select {
	case el := <-c.c:
		return el, nil
	case <-ctx.Done():
		return t, ctx.Err()
	}
}

func (c *cryChan[T]) Len() int {
	return len(c.c)
}

// dkgState represents the states of a DKG node. States change as follow:
//
//	┌───────┐          ┌───────┐
//	│Initial├─────────►│Sharing│
//	└───┬───┘          └───┬───┘
//	    │                  │
//	┌───▼─────┬─────►┌─────▼───┐
//	│Resharing│      │Certified│
//	└─────────┘◄─────┴─────────┘
type dkgState byte

func (s dkgState) String() string {
	switch s {
	case initial:
		return "Initial"
	case sharing:
		return "Sharing"
	case certified:
		return "Certified"
	case resharing:
		return "Resharing"
	default:
		return "UNKNOWN"
	}
}

const (
	initial dkgState = iota
	sharing
	certified
	resharing
)

// state is a struct contained in a handler that allows an actor to read the
// state of that handler. The actor should only use the getter functions to read
// the attributes.
type state struct {
	sync.Mutex
	// distrKey is only set once the node is certified
	distrKey kyber.Point
	// participants is set once a sharing or resharing starts
	participants []mino.Address
	pubkeys      []kyber.Point
	threshold    int
	dkgState     dkgState
}

func (s *state) switchState(new dkgState) error {
	s.Lock()
	defer s.Unlock()

	current := s.dkgState

	switch new {
	case initial:
		return xerrors.Errorf("initial state cannot be set manually")
	case sharing:
		if current != initial {
			return xerrors.Errorf("sharing state must switch from initial: %s", current)
		}
	case certified:
		if current != sharing && current != resharing {
			return xerrors.Errorf("certified state must switch from sharing or resharing: %s", current)
		}
	case resharing:
		if current != initial && current != certified {
			return xerrors.Errorf("resharing state must switch from initial or certified: %s", current)
		}
	}

	s.dkgState = new

	return nil
}

func (s *state) checkState(states ...dkgState) error {
	s.Lock()
	defer s.Unlock()

	current := s.dkgState

	for _, s := range states {
		if s == current {
			return nil
		}
	}

	return xerrors.Errorf("unexpected state: %s != one of %v", current, states)
}

func (s *state) Done() bool {
	s.Lock()
	defer s.Unlock()

	current := s.dkgState
	return current == certified
}

func (s *state) init(participants []mino.Address, pubkeys []kyber.Point, t int) {
	s.Lock()
	defer s.Unlock()

	s.participants = participants
	s.pubkeys = pubkeys
	s.threshold = t
}

func (s *state) getDistKey() kyber.Point {
	s.Lock()
	defer s.Unlock()
	return s.distrKey
}

func (s *state) setDistKey(key kyber.Point) {
	s.Lock()
	s.distrKey = key
	s.Unlock()
}

func (s *state) getParticipants() []mino.Address {
	s.Lock()
	defer s.Unlock()
	return s.participants
}

func (s *state) getPublicKeys() []kyber.Point {
	s.Lock()
	defer s.Unlock()
	return s.pubkeys
}

func (s *state) getThreshold() int {
	s.Lock()
	defer s.Unlock()
	return s.threshold
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
	log       zerolog.Logger
	running   bool
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address) *Handler {
	log := dela.Logger.With().Str("role", "DKG handler").Str("addr", me.String()).Logger()

	return &Handler{
		privKey: privKey,
		me:      me,
		startRes: &state{
			dkgState: initial,
		},
		log:     log,
		running: false,
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

	deals := newCryChan[types.Deal](200)
	responses := newCryChan[types.Response](100000)
	reshares := newCryChan[types.Reshare](200)

	globalCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	running := true
	runMux := sync.Mutex{}

	for {
		ctx, cancel := context.WithTimeout(context.Background(), recvTimeout)
		from, msg, err := in.Recv(ctx)
		cancel()

		if errors.Is(err, context.DeadlineExceeded) {
			runMux.Lock()
			if !running {
				h.log.Info().Msg("stream done, deadline exceeded")
				runMux.Unlock()
				return nil
			}
			runMux.Unlock()
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

		// We expect a Start message or a decrypt request at first, but we might
		// receive other messages in the meantime, like a Deal.
		switch msg := msg.(type) {

		case types.Start:
			go func() {
				err = h.start(globalCtx, msg, deals, responses, from, out)
				if err != nil {
					h.log.Err(err).Msg("failed to start")
				}

				runMux.Lock()
				running = false
				runMux.Unlock()
			}()

		case types.StartResharing:
			go func() {
				err = h.reshare(globalCtx, out, from, msg, reshares, responses)
				if err != nil {
					h.log.Err(err).Msg("failed to handle resharing")
				}

				runMux.Lock()
				running = false
				runMux.Unlock()
			}()

		case types.Deal:
			err = h.startRes.checkState(initial, sharing, certified, resharing)
			if err != nil {
				return xerrors.Errorf("bad state: %v", err)
			}

			deals.push(msg)

		case types.Reshare:
			err = h.startRes.checkState(initial, certified, resharing)
			if err != nil {
				return xerrors.Errorf("bad state: %v", err)
			}

			reshares.push(msg)

		case types.Response:
			err = h.startRes.checkState(initial, sharing, certified, resharing)
			if err != nil {
				return xerrors.Errorf("bad state: %v", err)
			}

			responses.push(msg)

		case types.DecryptRequest:
			err = h.startRes.checkState(certified)
			if err != nil {
				return xerrors.Errorf("bad state: %v", err)
			}

			return h.handleDecrypt(out, msg, from)

		case types.VerifiableDecryptRequest:
			err = h.startRes.checkState(certified)
			if err != nil {
				return xerrors.Errorf("bad state: %v", err)
			}

			return h.handleVerifiableDecrypt(out, msg, from)

		default:
			return xerrors.Errorf("expected Start message, decrypt request or "+
				"Deal as first message, got: %T", msg)
		}
	}
}

func (h *Handler) handleDecrypt(out mino.Sender, msg types.DecryptRequest,
	from mino.Address) error {

	if !h.startRes.Done() {
		return xerrors.Errorf("you must first initialize DKG. Did you call setup() first?")
	}

	S := suite.Point().Mul(h.privShare.V, msg.K)

	partial := suite.Point().Sub(msg.C, S)
	decryptReply := types.NewDecryptReply(int64(h.privShare.I), partial)

	errs := out.Send(decryptReply, from)
	err := <-errs
	if err != nil {
		return xerrors.Errorf("got an error while sending the decrypt reply: %v", err)
	}

	return nil
}

// start is called when the node has received its start message. Note that we
// might have already received some deals from other nodes in the meantime. The
// function handles the DKG creation protocol.
func (h *Handler) start(ctx context.Context, start types.Start, deals cryChan[types.Deal],
	resps cryChan[types.Response], from mino.Address, out mino.Sender) error {

	err := h.startRes.switchState(sharing)
	if err != nil {
		return xerrors.Errorf("failed to switch state: %v", err)
	}

	if len(start.GetAddresses()) != len(start.GetPublicKeys()) {
		return xerrors.Errorf("there should be as many participants as "+
			"pubKey: %d != %d", len(start.GetAddresses()), len(start.GetPublicKeys()))
	}

	// create the DKG
	t := threshold.ByzantineThreshold(len(start.GetPublicKeys()))
	d, err := pedersen.NewDistKeyGenerator(suite, h.privKey, start.GetPublicKeys(), t)
	if err != nil {
		return xerrors.Errorf("failed to create new DKG: %v", err)
	}

	h.dkg = d

	h.startRes.init(start.GetAddresses(), start.GetPublicKeys(), start.GetThreshold())

	err = h.doDKG(ctx, deals, resps, out, from)
	if err != nil {
		xerrors.Errorf("something went wrong during DKG: %v", err)
	}

	return nil
}

// doDKG calls the subsequent DKG steps
func (h *Handler) doDKG(ctx context.Context, deals cryChan[types.Deal],
	resps cryChan[types.Response], out mino.Sender, from mino.Address) error {

	defer func() {
		h.Lock()
		h.running = false
		h.Unlock()
	}()

	h.log.Info().Str("action", "deal").Msg(newState)
	err := h.deal(ctx, out)
	if err != nil {
		return xerrors.Errorf("failed to deal: %v", err)
	}

	h.log.Info().Str("action", "respond").Msg(newState)
	err = h.respond(ctx, deals, out)
	if err != nil {
		return xerrors.Errorf("failed to respond: %v", err)
	}

	h.log.Info().Str("action", "certify").Msg(newState)
	numResps := (len(h.startRes.getParticipants()) - 1) * (len(h.startRes.getParticipants()) - 1)
	err = h.certify(ctx, resps, numResps)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	err = h.startRes.switchState(certified)
	if err != nil {
		return xerrors.Errorf("failed to switch state: %v", err)
	}

	h.log.Info().Str("action", "finalize").Msg(newState)
	err = h.finalize(ctx, from, out)
	if err != nil {
		return xerrors.Errorf("failed to finalize: %v", err)
	}

	h.log.Info().Str("action", "done").Msg(newState)

	return nil
}

func (h *Handler) deal(ctx context.Context, out mino.Sender) error {
	// Send my Deals to the other nodes. Note that we take an optimistic
	// approach and expect nodes to always accept messages. If not, the protocol
	// can hang forever.

	deals, err := h.dkg.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

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

		to := h.startRes.getParticipants()[i]

		h.log.Trace().Str("to", to.String()).Msg("send deal")

		errs := out.Send(dealMsg, to)

		// this can be blocking if the recipient is not receiving message
		select {
		case err = <-errs:
			if err != nil {
				h.log.Err(err).Str("to", to.String()).Msg("failed to send deal")
			}
		case <-ctx.Done():
			return xerrors.Errorf("context done: %v", ctx.Err())
		}
	}

	return nil
}

func (h *Handler) respond(ctx context.Context, deals cryChan[types.Deal], out mino.Sender) error {
	numReceivedDeals := 0

	for numReceivedDeals < len(h.startRes.getParticipants())-1 {
		deal, err := deals.pop(ctx)
		if err != nil {
			return xerrors.Errorf("context done: %v", err)
		}

		err = h.handleDeal(ctx, deal, out, h.startRes.getParticipants())
		if err != nil {
			return xerrors.Errorf("failed to handle received deal: %v", err)
		}

		numReceivedDeals++

		h.log.Trace().Str("total", strconv.Itoa(numReceivedDeals)).Msg("deal received")
	}

	return nil
}

// certify collects the responses and checks if the node is certified. The
// number of expected responses depends on the case:
//   - Basic setup: (n_participants-1)^2, because each node won't broadcast the
//     certification of their own deals, and they won't sent to themselves their
//     own deal certification either
//   - Resharing with leaving or joining node: (n_common + (n_new - 1)) * n_old,
//     nodes that are doing a resharing will broadcast their own deals
//   - Resharing with staying node: (n_common + n_new) * n_old
func (h *Handler) certify(ctx context.Context, resps cryChan[types.Response], expected int) error {

	responsesReceived := 0

	for responsesReceived < expected {
		msg, err := resps.pop(ctx)
		if err != nil {
			return xerrors.Errorf("context done: %v", err)
		}

		resp := pedersen.Response{
			Index: msg.GetIndex(),
			Response: &vss.Response{
				SessionID: msg.GetResponse().GetSessionID(),
				Index:     msg.GetResponse().GetIndex(),
				Status:    msg.GetResponse().GetStatus(),
				Signature: msg.GetResponse().GetSignature(),
			},
		}

		_, err = h.dkg.ProcessResponse(&resp)
		if err != nil {
			return xerrors.Errorf("failed to process response: %v", err)
		}

		responsesReceived++

		h.log.Trace().Int("total", responsesReceived).Msg("response processed")
	}

	if !h.dkg.Certified() {
		return xerrors.New("node is not certified")
	}

	return nil
}

// finalize saves the result and announces it to the orchestrator.
func (h *Handler) finalize(ctx context.Context, from mino.Address, out mino.Sender) error {
	// Send back the public DKG key
	distKey, err := h.dkg.DistKeyShare()
	if err != nil {
		return xerrors.Errorf("failed to get distr key: %v", err)
	}

	// Update the state before sending the acknowledgement to the orchestrator,
	// so that it can process decrypt requests right away.
	h.startRes.setDistKey(distKey.Public())

	h.Lock()
	h.privShare = distKey.PriShare()
	h.Unlock()

	done := types.NewStartDone(distKey.Public())

	select {
	case err = <-out.Send(done, from):
		if err != nil {
			return xerrors.Errorf("got an error while sending pub key: %v", err)
		}
	case <-ctx.Done():
		h.log.Warn().Msgf("context done (this might be expected): %v", ctx.Err())
	}

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (h *Handler) handleDeal(ctx context.Context, msg types.Deal,
	out mino.Sender, to []mino.Address) error {

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

	response, err := h.dkg.ProcessDeal(deal)
	if err != nil {
		return xerrors.Errorf("failed to process deal: %v", err)
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

	for _, addr := range to {
		if addr.Equal(h.me) {
			continue
		}

		h.log.Trace().Str("to", addr.String()).Uint32("dealer", response.Index).
			Msg("sending response")

		errs := out.Send(resp, addr)

		select {
		case err = <-errs:
			if err != nil {
				return xerrors.Errorf("failed to send response to '%s': %v", addr, err)
			}
		case <-ctx.Done():
			return xerrors.Errorf("context done: %v", ctx.Err())
		}
	}

	return nil
}

func (h *Handler) finalizeReshare(ctx context.Context, isCommonNode bool, out mino.Sender, from mino.Address) error {
	// Send back the public DKG key
	publicKey := h.startRes.getDistKey()

	// if the node is new or a common node it should update its state
	if publicKey == nil || isCommonNode {
		distrKey, err := h.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("failed to get distr key: %v", err)
		}

		publicKey = distrKey.Public()

		// Update the state before sending to acknowledgement to the
		// orchestrator, so that it can process decrypt requests right away.
		h.startRes.setDistKey(distrKey.Public())
		h.Lock()
		h.privShare = distrKey.PriShare()
		h.Unlock()
	}

	// all the old, new and common nodes should announce their public key to the
	// initiator, in this way the initiator can make sure that every body has
	// finished the resharing successfully
	done := types.NewStartDone(publicKey)

	select {
	case err := <-out.Send(done, from):
		if err != nil {
			return xerrors.Errorf("got an error while sending pub key: %v", err)
		}
	case <-ctx.Done():
		h.log.Warn().Msgf("context done (this might be expected): %v", ctx.Err())
	}

	dela.Logger.Info().Msgf("%s announced the DKG public key", h.me)

	return nil
}

// reshare handles the resharing request. Acts differently for the new
// and old and common nodes
func (h *Handler) reshare(ctx context.Context, out mino.Sender,
	from mino.Address, msg types.StartResharing, reshares cryChan[types.Reshare], resps cryChan[types.Response]) error {

	err := h.startRes.switchState(resharing)
	if err != nil {
		return xerrors.Errorf("failed to switch state: %v", err)
	}

	addrsNew := msg.GetAddrsNew()

	if len(addrsNew) != len(msg.GetPubkeysNew()) {
		return xerrors.Errorf("there should be as many participants as pubKey: %d != %d",
			len(addrsNew), len(msg.GetPubkeysNew()))
	}

	err = h.doReshare(ctx, msg, from, out, reshares, resps)
	if err != nil {
		xerrors.Errorf("failed to reshare: %v", err)
	}

	return nil
}

// doReshare is called when the node has received its reshare message. Note that
// we might have already received some deals from other nodes in the meantime.
// The function handles the DKG resharing protocol.
func (h *Handler) doReshare(ctx context.Context, start types.StartResharing,
	from mino.Address, out mino.Sender, reshares cryChan[types.Reshare], resps cryChan[types.Response]) error {

	h.log.Info().Msgf("resharing with %v", start.GetAddrsNew())

	var expectedResponses int
	addrsOld := h.startRes.getParticipants()
	addrsNew := start.GetAddrsNew()

	isOldNode := h.startRes.getDistKey() != nil
	isNewNode := !isOldNode
	isCommonNode := isOldNode && isInSlice(h.me, addrsNew) && isInSlice(h.me, addrsOld)

	if isOldNode {
		// 1. Update local DKG for resharing
		share, err := h.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("failed to create : %v", err)
		}

		h.log.Trace().Msgf("old node: %v", h.startRes.getPublicKeys())

		c := &pedersen.Config{
			Suite:        suite,
			Longterm:     h.privKey,
			OldNodes:     h.startRes.getPublicKeys(),
			NewNodes:     start.GetPubkeysNew(),
			Share:        share,
			Threshold:    start.GetTNew(),
			OldThreshold: h.startRes.getThreshold(),
		}

		d, err := pedersen.NewDistKeyHandler(c)
		if err != nil {
			return xerrors.Errorf("failed to compute the new dkg: %v", err)
		}

		h.dkg = d

		// 2. Send my Deals to the new and common nodes
		err = h.sendDealsResharing(ctx, out, addrsNew, share.Commits)
		if err != nil {
			return xerrors.Errorf("failed to send deals: %v", err)
		}
	}

	// If the node is a new node or is a common node it should do the next steps
	if isNewNode || isCommonNode {
		// 1. Process the incoming deals
		err := h.receiveDealsResharing(ctx, isCommonNode, start, out, reshares)
		if err != nil {
			return xerrors.Errorf("failed to receive deals: %v", err)
		}

		// Save the specifications of the new committee in the handler state
		h.startRes.init(start.GetAddrsNew(), start.GetPubkeysNew(), start.GetTNew())
	}

	// Note that a node can be old and common
	if isOldNode {
		expectedResponses = (start.GetTNew()) * len(h.startRes.getParticipants())
	}
	if isCommonNode {
		expectedResponses = (start.GetTNew() - 1) * len(addrsOld)
	}
	if isNewNode {
		expectedResponses = (start.GetTNew() - 1) * start.GetTOld()
	}

	// All nodes should certify.
	err := h.certify(ctx, resps, expectedResponses)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	err = h.startRes.switchState(certified)
	if err != nil {
		return xerrors.Errorf("failed to switch state: %v", err)
	}

	// Announce the DKG public key All the old, new and common nodes would
	// announce the public key. In this way the initiator can make sure the
	// resharing was completely successful.
	err = h.finalizeReshare(ctx, isCommonNode, out, from)
	if err != nil {
		return xerrors.Errorf("failed to announce dkg public key: %v", err)
	}

	return nil
}

// sendDealsResharing is similar to sendDeals except that it creates
// dealResharing which has more data than Deal. Only the old nodes call this
// function.
func (h *Handler) sendDealsResharing(ctx context.Context, out mino.Sender,
	participants []mino.Address, publicCoeff []kyber.Point) error {

	h.log.Trace().Msgf("%v is generating its deals", h.me)

	deals, err := h.dkg.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

	h.log.Trace().Msgf("%s is sending its deals", h.me)

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

		//dealResharing contains the public coefficients as well
		dealResharingMsg := types.NewReshare(dealMsg, publicCoeff)

		h.log.Trace().Msgf("%s sent dealResharing %d", h.me, i)

		select {
		case err := <-out.Send(dealResharingMsg, participants[i]):
			if err != nil {
				return xerrors.Errorf("failed to send resharing deal: %v", err)
			}
		case <-ctx.Done():
			return xerrors.Errorf("context done: %v", ctx.Err())
		}
	}

	h.log.Debug().Msgf("%s sent all its deals", h.me)

	return nil
}

// receiveDealsResharing is similar to receiveDeals except that it receives the
// dealResharing. Only the new or common nodes call this function
func (h *Handler) receiveDealsResharing(ctx context.Context, isCommonNode bool,
	resharingRequest types.StartResharing, out mino.Sender, reshares cryChan[types.Reshare]) error {

	h.log.Trace().Msgf("%v is handling deals from other nodes", h.me)

	addrsNew := resharingRequest.GetAddrsNew()
	var addrsOld []mino.Address

	numReceivedDeals := 0
	expectedDeals := 0

	// by default we read the old addresses from the state
	addrsOld = h.startRes.getParticipants()

	// the new nodes don't have the old addresses in their state
	if !isCommonNode {
		addrsOld = resharingRequest.GetAddrsOld()
	}

	expectedDeals = len(addrsOld)

	// we find the union of the old and new address sets to avoid from sending a
	// message to the common nodes multiple times
	addrsAll := union(addrsNew, addrsOld)

	for numReceivedDeals < expectedDeals {
		reshare, err := reshares.pop(ctx)
		if err != nil {
			return xerrors.Errorf("context done: %v", err)
		}

		if !isCommonNode && numReceivedDeals == 0 {

			c := &pedersen.Config{
				Suite:        suite,
				Longterm:     h.privKey,
				OldNodes:     resharingRequest.GetPubkeysOld(),
				NewNodes:     resharingRequest.GetPubkeysNew(),
				PublicCoeffs: reshare.GetPublicCoeffs(),
				Threshold:    resharingRequest.GetTNew(),
				OldThreshold: resharingRequest.GetTOld(),
			}

			newDkg, err := pedersen.NewDistKeyHandler(c)
			if err != nil {
				return xerrors.Errorf("failed to create the new dkg for %s: %v", h.me, err)
			}

			h.dkg = newDkg
		}

		deal := reshare.GetDeal()

		err = h.handleDeal(ctx, deal, out, addrsAll)
		if err != nil {
			return xerrors.Errorf("failed to handle received deal: %v", err)
		}

		numReceivedDeals++
	}

	h.log.Debug().Msgf("%v received all the expected deals", h.me)

	return nil
}

func (h *Handler) handleVerifiableDecrypt(out mino.Sender,
	msg types.VerifiableDecryptRequest, from mino.Address) error {

	type job struct {
		index int // index where to put the response
		ct    types.Ciphertext
	}

	ciphertexts := msg.GetCiphertexts()
	batchsize := len(ciphertexts)

	wgBatchReply := sync.WaitGroup{}

	shareAndProofs := make([]types.ShareAndProof, batchsize)
	jobChan := make(chan job)

	// Fill the chan with jobs
	go func() {
		for i, ct := range ciphertexts {
			jobChan <- job{
				index: i,
				ct:    ct,
			}

		}
		close(jobChan)
	}()

	n := workerNum
	if batchsize < n {
		n = batchsize
	}

	// Spins up workers to process jobs from the chan
	for i := 0; i < n; i++ {
		wgBatchReply.Add(1)
		go func() {
			defer wgBatchReply.Done()

			for j := range jobChan {
				sp, err := h.verifiableDecryption(j.ct)
				if err != nil {
					h.log.Err(err).Msg("verifiable decryption failed")
				}

				shareAndProofs[j.index] = *sp
			}

		}()
	}

	wgBatchReply.Wait()

	h.log.Info().Msg("sending back verifiable decrypt reply")

	verifiableDecryptReply := types.NewVerifiableDecryptReply(shareAndProofs)

	errs := out.Send(verifiableDecryptReply, from)
	err := <-errs
	if err != nil {
		return xerrors.Errorf("failed to send verifiable decrypt: %v", err)
	}

	return nil
}

// checkEncryptionProof verifies the encryption proofs.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func (h *Handler) checkEncryptionProof(cp types.Ciphertext) error {

	tmp1 := suite.Point().Mul(cp.F, nil)
	tmp2 := suite.Point().Mul(cp.E, cp.K)
	w := suite.Point().Sub(tmp1, tmp2)

	tmp1 = suite.Point().Mul(cp.F, cp.GBar)
	tmp2 = suite.Point().Mul(cp.E, cp.UBar)
	wBar := suite.Point().Sub(tmp1, tmp2)

	hash := sha256.New()
	cp.C.MarshalTo(hash)
	cp.K.MarshalTo(hash)
	cp.UBar.MarshalTo(hash)
	w.MarshalTo(hash)
	wBar.MarshalTo(hash)

	tmp := suite.Scalar().SetBytes(hash.Sum(nil))
	if !tmp.Equal(cp.E) {
		return xerrors.Errorf("hash not valid: %x != %x", cp.E, tmp)
	}

	return nil
}

// verifiableDecryption generates the decryption shares as well as the
// decryption proof.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func (h *Handler) verifiableDecryption(ct types.Ciphertext) (*types.ShareAndProof, error) {
	err := h.checkEncryptionProof(ct)
	if err != nil {
		return nil, xerrors.Errorf("failed to check proof: %v", err)
	}

	h.RLock()
	ui := suite.Point().Mul(h.privShare.V, ct.K)
	h.RUnlock()

	// share of this party, needed for decrypting
	partial := suite.Point().Sub(ct.C, ui)

	si := suite.Scalar().Pick(suite.RandomStream())
	UHat := suite.Point().Mul(si, ct.K)
	HHat := suite.Point().Mul(si, nil)

	hash := sha256.New()
	ui.MarshalTo(hash)
	UHat.MarshalTo(hash)
	HHat.MarshalTo(hash)
	Ei := suite.Scalar().SetBytes(hash.Sum(nil))

	h.RLock()
	Fi := suite.Scalar().Add(si, suite.Scalar().Mul(Ei, h.privShare.V))
	Hi := suite.Point().Mul(h.privShare.V, nil)
	h.RUnlock()

	h.RLock()
	sp := types.ShareAndProof{
		V:  partial,
		I:  int64(h.privShare.I),
		Ui: ui,
		Ei: Ei,
		Fi: Fi,
		Hi: Hi,
	}
	h.RUnlock()

	return &sp, nil
}

// isInSlice gets an address and a slice of addresses and returns true if that
// address is in the slice. This function is called for checking whether an old
// committee member is in the new committee as well or not
func isInSlice(addr mino.Address, addrs []mino.Address) bool {
	for _, other := range addrs {
		if addr.Equal(other) {
			return true
		}
	}

	return false
}

// union performs a union of el1 and el2.
func union(el1 []mino.Address, el2 []mino.Address) []mino.Address {
	addrsAll := el1

	for _, other := range el2 {
		exist := false
		for _, addr := range el1 {
			if addr.Equal(other) {
				exist = true
				break
			}
		}
		if !exist {
			addrsAll = append(addrsAll, other)
		}
	}

	return addrsAll
}
