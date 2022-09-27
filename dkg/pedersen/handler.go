package pedersen

import (
	"context"
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
const recvTimeout = time.Second * 4

// the channel size used to stored buffered deals and responses. This arbitrary
// value must be set according to the characteristic of the system: number of
// nodes, networking, memory, etc...
const chanSize = 200

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

// state is a struct contained in a handler that allows an actor to read the
// state of that handler. The actor should only use the getter functions to read
// the attributes.
type state struct {
	sync.Mutex
	starting     bool
	resharing    bool
	distrKey     kyber.Point
	participants []mino.Address
	publicKeys   []kyber.Point
	threshold    int
}

func (s *state) IsStarting() bool {
	s.Lock()
	defer s.Unlock()
	return s.starting
}

func (s *state) Start() {
	s.Lock()
	defer s.Unlock()
	s.starting = true
}

func (s *state) AbortStart() {
	s.Lock()
	defer s.Unlock()
	s.starting = false
}

func (s *state) IsReshare() bool {
	s.Lock()
	defer s.Unlock()
	return s.resharing
}

func (s *state) Reshare() {
	s.Lock()
	defer s.Unlock()
	s.resharing = true
}

func (s *state) FinishReshare() {
	s.Lock()
	defer s.Unlock()
	s.resharing = false
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

func (s *state) GetPublicKeys() []kyber.Point {
	s.Lock()
	defer s.Unlock()
	return s.publicKeys
}

func (s *state) SetPublicKeys(publicKeys []kyber.Point) {
	s.Lock()
	s.publicKeys = publicKeys
	s.Unlock()
}

func (s *state) GetThreshold() int {
	s.Lock()
	defer s.Unlock()
	return s.threshold
}

func (s *state) SetThreshold(threshold int) {
	s.Lock()
	s.threshold = threshold
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
	log       zerolog.Logger
	running   bool
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address) *Handler {
	log := dela.Logger.With().Str("role", "DKG handler").Str("addr", me.String()).Logger()

	return &Handler{
		privKey:  privKey,
		me:       me,
		startRes: &state{},
		log:      log,
		running:  false,
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

	// We make sure not additional request is accepted if a setup is in
	// progress.
	h.Lock()
	if h.running {
		h.Unlock()
		return xerrors.Errorf("DKG is running")
	}
	if !h.startRes.Done() {
		// This is the first setup
		h.running = true
	}
	h.Unlock()

	deals := newCryChan[types.Deal](chanSize)
	responses := newCryChan[types.Response](chanSize)
	reshares := newCryChan[types.Reshare](chanSize)

	globalCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), recvTimeout)
		from, msg, err := in.Recv(ctx)
		cancel()

		if errors.Is(err, context.DeadlineExceeded) {
			if h.startRes.Done() {
				return nil
			}

			continue
		}

		if errors.Is(err, io.EOF) {
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
			err := h.start(globalCtx, msg, deals, responses, from, out)
			if err != nil {
				return xerrors.Errorf("failed to start: %v", err)
			}

		case types.ResharingRequest:
			dela.Logger.Trace().Msgf("%v received resharing request from %v\n", h.me, from)

			done := make(chan struct{})

			err = h.handleReshare(out, from, msg, done, reshares, responses)
			if err != nil {
				return xerrors.Errorf("failed to handle resharing: %v", err)
			}
			go func() {
				<-done
				if h.startRes.Done() {
					cancel()
				}
			}()

		case types.Deal:
			deals.push(msg)

		case types.Reshare:
			dela.Logger.Trace().Msgf("%v received resharing deal from %v\n", h.me, from)

			reshares.push(msg)

		case types.Response:
			responses.push(msg)
			h.log.Info().Int("total", responses.Len()).Msg("pushing a response")

		case types.DecryptRequest:
			dela.Logger.Trace().Msgf("%v received decrypt request from %v", h.me, from)

			return h.handleDecrypt(out, msg, from)

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

	h.RLock()
	S := suite.Point().Mul(h.privShare.V, msg.K)
	h.RUnlock()

	partial := suite.Point().Sub(msg.C, S)

	h.RLock()
	decryptReply := types.NewDecryptReply(int64(h.privShare.I), partial)
	h.RUnlock()

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

	h.startRes.SetParticipants(start.GetAddresses())

	// asynchronously start the procedure. This allows for receiving messages
	// in the main for loop in the meantime.
	go func() {
		err = h.doDKG(ctx, deals, resps, out, from)
		if err != nil {
			h.log.Err(err).Msg("something went wrong during DKG")
		}
	}()

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
	err = h.certify(ctx, resps, out)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
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

		to := h.startRes.participants[i]

		h.log.Info().Str("to", to.String()).Msg("send deal")

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

	for numReceivedDeals < len(h.startRes.participants)-1 {
		deal, err := deals.pop(ctx)
		if err != nil {
			return xerrors.Errorf("context done: %v", err)
		}

		err = h.handleDeal(ctx, deal, out, h.startRes.GetParticipants())
		if err != nil {
			return xerrors.Errorf("failed to handle received deal: %v", err)
		}

		numReceivedDeals++

		h.log.Info().Str("total", strconv.Itoa(numReceivedDeals)).Msg("deal received")
	}

	return nil
}

func (h *Handler) certify(ctx context.Context, resps cryChan[types.Response], out mino.Sender) error {

	responsesReceived := 0
	expected := (len(h.startRes.participants) - 1) * (len(h.startRes.participants) - 1)

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

		h.log.Info().Int("total", responsesReceived).Msg("response processed")
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

	// Update the state before sending the acknowledgement to the
	// orchestrator, so that it can process decrypt requests right away.
	h.startRes.SetDistKey(distKey.Public())

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
		return xerrors.Errorf("context done: %v", ctx.Err())
	}

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (h *Handler) handleDeal(ctx context.Context, msg types.Deal, out mino.Sender, to []mino.Address) error {

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

		h.log.Info().Str("to", addr.String()).Str("dealer", strconv.Itoa(int(response.Index))).Msg("sending response")

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

	dela.Logger.Debug().Msgf("%s is certified", h.me)

	return nil
}

func (h *Handler) announceDkgPublicKey(isCommonNode bool, out mino.Sender, from mino.Address) error {
	// 6. Send back the public DKG key

	var publicKey kyber.Point

	isOldNode := h.startRes.distrKey != nil
	isNewNode := !isOldNode

	// if the node is new or a common node it should update its state
	if isNewNode || isCommonNode {
		distrKey, err := h.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("failed to get distr key: %v", err)
		}
		publicKey = distrKey.Public()
		// 7. Update the state before sending to acknowledgement to the
		// orchestrator, so that it can process decrypt requests right away.
		h.startRes.SetDistKey(distrKey.Public())
		h.Lock()
		h.privShare = distrKey.PriShare()
		h.Unlock()
	} else {
		publicKey = h.startRes.GetDistKey()
	}

	// all the old, new and common nodes should announce their public key to the initiator,
	// in this way the initiator can make sure that every body has finished the resharing successfully
	done := types.NewStartDone(publicKey)
	err := <-out.Send(done, from)
	if err != nil {
		return xerrors.Errorf("got an error while sending pub key: %v", err)
	}

	dela.Logger.Info().Msgf("%s announced the DKG public key", h.me)

	return nil
}

// handleReshare handles the resharing request. Acts differently for the new
// and old and common nodes
func (h *Handler) handleReshare(out mino.Sender,
	from mino.Address, msg types.ResharingRequest, done chan struct{}, reshares cryChan[types.Reshare], resps cryChan[types.Response]) error {

	if h.startRes.IsReshare() {
		dela.Logger.Warn().Msgf(
			"%v ignored reshare request from %v as it "+
				" is already resharing\n", h.me, from)
		return xerrors.Errorf("dkg is already resharing")
	}

	h.startRes.Reshare()

	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, 5*time.Minute)
	go func() {
		err := h.reshare(ctx, msg, from, out, reshares, resps)
		if err != nil {
			dela.Logger.Error().Msgf(
				"%v failed to reshare: %v", h.me, err)
			h.startRes.FinishReshare()
		}
		close(done)
	}()

	return nil
}

// reshare is called when the node has received its reshare message. Note that
// we might have already received some deals from other nodes in the meantime.
// The function handles the DKG resharing protocol.
func (h *Handler) reshare(ctx context.Context, resharingRequest types.ResharingRequest,
	from mino.Address, out mino.Sender, reshares cryChan[types.Reshare], resps cryChan[types.Response]) error {

	dela.Logger.Info().Msgf("%v is resharing", h.me)

	addrsNew := resharingRequest.GetAddrsNew()

	if len(addrsNew) != len(resharingRequest.GetPubkeysNew()) {
		return xerrors.Errorf(
			"there should be as many participants as "+
				"pubKey: %d != %d", len(addrsNew),
			len(resharingRequest.GetPubkeysNew()),
		)
	}

	isOldNode := h.startRes.distrKey != nil
	isNewNode := !isOldNode

	// By default the node is not common. Later we check
	isCommonNode := false

	// If the node is in the old committee, it should do the following steps
	if isOldNode {
		addrsOld := h.startRes.GetParticipants()

		// This variable is true if the node is common between the old and the new committee
		isCommonNode = isInSlice(h.me, addrsNew) && isInSlice(h.me, addrsOld)

		// 1. Update mydkg for resharing
		share, err := h.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("failed to create : %v", err)
		}

		c := &pedersen.Config{
			Suite:        suite,
			Longterm:     h.privKey,
			OldNodes:     h.startRes.GetPublicKeys(),
			NewNodes:     resharingRequest.GetPubkeysNew(),
			Share:        share,
			Threshold:    resharingRequest.GetTNew(),
			OldThreshold: h.startRes.threshold,
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
		err := h.receiveDealsResharing(ctx, isCommonNode, resharingRequest, out, reshares)
		if err != nil {
			return xerrors.Errorf("failed to receive deals: %v", err)
		}

		// Save the specifications of the new committee in the handler state
		h.startRes.SetParticipants(resharingRequest.GetAddrsNew())
		h.startRes.SetPublicKeys(resharingRequest.GetPubkeysNew())
		h.startRes.SetThreshold(resharingRequest.TNew)
	}

	// 4. Certify
	// all the nodes should certify
	err := h.certify(ctx, resps, out)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	// 5. Announce the DKG public key All the old, new and common nodes would
	// announce the public key. In this way the initiator can make sure the
	// resharing was completely successful.
	err = h.announceDkgPublicKey(isCommonNode, out, from)
	if err != nil {
		return xerrors.Errorf("failed to announce dkg public key: %v", err)
	}

	h.startRes.FinishReshare()

	return nil
}

// sendDealsResharing is similar to sendDeals except that it creates
// dealResharing which has more data than Deal. Only the old nodes call this
// function
func (h *Handler) sendDealsResharing(ctx context.Context, out mino.Sender,
	participants []mino.Address, publicCoeff []kyber.Point) error {

	dela.Logger.Trace().Msgf("%v is generating its deals", h.me)

	deals, err := h.dkg.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

	dela.Logger.Trace().Msgf("%s is sending its deals", h.me)

	done := make(chan int)
	errors := make(chan error)

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

		dela.Logger.Trace().Msgf("%s sent dealResharing %d", h.me, i)
		errch := out.Send(dealResharingMsg, participants[i])

		// this should be further improved by using a worker pool,
		// as opposed to an unbounded number of goroutines,
		// but for the time being is okay-ish. -- 2022/08/09
		go func(errs <-chan error, idx int) {
			err := <-errs
			if err != nil {
				dela.Logger.Warn().Msgf(
					"got an error while sending dealResharing %v: %v", i, err)
				errors <- err
			} else {
				done <- idx
			}
		}(errch, i)
	}

	sent := 0
	for sent < len(deals) {
		select {
		case idx := <-done:
			dela.Logger.Trace().Msgf("%s sent deal resharing %v", h.me, idx)
			sent++

		case err := <-errors:
			dela.Logger.Error().Msgf("%s failed sending a deal: %v", h.me, err)
			return xerrors.Errorf("failed sending a deal: %v", err)

		case <-ctx.Done():
			dela.Logger.Error().Msgf("%s timed out while sending deals", h.me)
			return xerrors.Errorf("timed out while sending deals")
		}
	}

	dela.Logger.Debug().Msgf("%s sent all its deals", h.me)

	return nil
}

// receiveDealsResharing is similar to receiveDeals except that it receives the
// dealResharing. Only the new or common nodes call this function
func (h *Handler) receiveDealsResharing(ctx context.Context, isCommonNode bool, resharingRequest types.ResharingRequest, out mino.Sender, reshares cryChan[types.Reshare]) error {
	dela.Logger.Trace().Msgf("%v is handling deals from other nodes", h.me)

	addrsNew := resharingRequest.GetAddrsNew()
	var addrsOld []mino.Address

	numReceivedDeals := 0
	expectedDeals := 0

	// by default we read the old addresses from the state
	addrsOld = h.startRes.GetParticipants()

	// the new nodes don't have the old addresses in their state
	if !isCommonNode {
		addrsOld = resharingRequest.GetAddrsOld()
	}

	expectedDeals = len(addrsOld)

	// we find the union of the old and new address sets to aviod from sending a
	// message to the common nodes multiple times
	addrsAll := unionOfTwoSlices(addrsNew, addrsOld)

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
				return xerrors.Errorf("failed to create the new dkg for %s: %v",
					h.me, err)
			}

			h.dkg = newDkg
		}

		deal := reshare.GetDeal()

		err = h.handleDeal(ctx, deal, out, addrsAll)
		if err != nil {
			dela.Logger.Warn().Msgf("%s failed to handle deal: %v", h.me, err)
			return xerrors.Errorf("failed to handle received deal: %v", err)
		}

		numReceivedDeals++
	}

	dela.Logger.Debug().Msgf("%v received all the expected deals", h.me)

	return nil
}

///////// auxiliary functions

// isInSlice gets an address and a slice of addresses and returns true if that
// address is in the slice. This function is called for cheking whether an old
// committee member is in the new commmitte as well or not
func isInSlice(addr mino.Address, addrs []mino.Address) bool {

	for _, other := range addrs {
		if addr.Equal(other) {
			return true
		}
	}

	return false
}

// unionOfTwoSlices gets the list of the old committee members and new committee
// members and returns the union.
func unionOfTwoSlices(addrsSlice1 []mino.Address, addrsSlice2 []mino.Address) []mino.Address {
	addrsAll := addrsSlice1

	for _, other := range addrsSlice2 {
		exist := false
		for _, addr := range addrsSlice1 {
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
