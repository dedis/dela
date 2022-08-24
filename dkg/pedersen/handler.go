package pedersen

import (
	"context"
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
	dkg            *pedersen.DistKeyGenerator
	privKey        kyber.Scalar
	me             mino.Address
	privShare      *share.PriShare
	startRes       *state
	deals          chan dealFrom
	dealsResharing chan dealFromResharing
	responses      chan responseFrom
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address) *Handler {
	return &Handler{
		privKey:        privKey,
		me:             me,
		startRes:       &state{},
		deals:          make(chan dealFrom, 1000),
		dealsResharing: make(chan dealFromResharing, 1000),
		responses:      make(chan responseFrom, 1000),
	}
}

type dealFrom struct {
	deal *types.Deal
	from mino.Address
}

type dealFromResharing struct {
	dealResharing *types.DealResharing
	from          mino.Address
}

type responseFrom struct {
	response *types.Response
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		from, msg, err := in.Recv(ctx)
		if err != nil {
			if h.startRes.Done() {
				// successfully started, terminate cleanly
				return nil
			}
			return xerrors.Errorf("failed to receive: %v", err)
		}

		dela.Logger.Trace().Msgf("%v received message from %v\n", h.me, from)

		// We expect a Start message or a decrypt request at first, but we might
		// receive other messages in the meantime, like a Deal.
		switch msg := msg.(type) {

		case types.Start:
			dela.Logger.Trace().Msgf("%v received start from %v\n", h.me, from)
			done := make(chan struct{})
			err = h.handleStart(out, from, msg, done)
			if err != nil {
				return xerrors.Errorf("failed to handle start: %v", err)
			}
			go func() {
				<-done
				if h.startRes.Done() {
					cancel()
				}
			}()

		case types.ResharingRequest:
			dela.Logger.Trace().Msgf("%v received resharing request from %v\n", h.me, from)

			done := make(chan struct{})

			err = h.handleReshare(out, from, msg, done)
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
			dela.Logger.Trace().Msgf("%v received deal from %v\n", h.me, from)

			h.deals <- dealFrom{
				&msg,
				from,
			}

		case types.DealResharing:
			dela.Logger.Trace().Msgf("%v received resharing deal from %v\n", h.me, from)

			h.dealsResharing <- dealFromResharing{
				&msg,
				from,
			}

		case types.Response:
			dela.Logger.Trace().Msgf("%v received response from %v\n", h.me, from)

			h.responses <- responseFrom{
				&msg,
				from,
			}

		case types.DecryptRequest:
			dela.Logger.Trace().Msgf("%v received decrypt request from %v\n", h.me, from)

			return h.handleDecrypt(out, msg, from)

		default:
			dela.Logger.Error().Msgf(
				"%v received an unsupported message %v from %v\n", h.me,
				msg, from)
			return xerrors.Errorf("expected Start message, decrypt request or "+
				"Deal as first message, got: %T", msg)
		}
	}
}

func (h *Handler) handleDecrypt(out mino.Sender, msg types.DecryptRequest,
	from mino.Address) error {
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
	err := <-errs
	if err != nil {
		return xerrors.Errorf(
			"got an error while sending the decrypt "+
				"reply: %v", err,
		)
	}
	return nil
}

func (h *Handler) handleStart(out mino.Sender,
	from mino.Address, msg types.Start, done chan struct{}) error {
	if h.startRes.Done() {
		dela.Logger.Warn().Msgf(
			"%v ignored start request from %v as it is already"+
				" started\n", h.me, from)
		return xerrors.Errorf("dkg is already started")
	}

	if h.startRes.IsStarting() {
		dela.Logger.Warn().Msgf(
			"%v ignored start request from %v as it is already"+
				" starting\n", h.me, from)
		return xerrors.Errorf("dkg is already starting")
	}

	h.startRes.Start()

	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, 5*time.Minute)
	go func() {
		err := h.start(ctx, msg, from, out)
		if err != nil {
			dela.Logger.Error().Msgf(
				"%v failed to start: %v", h.me, err)
			h.startRes.AbortStart()
		}
		close(done)
	}()

	return nil
}

// start is called when the node has received its start message. Note that we
// might have already received some deals from other nodes in the meantime. The
// function handles the DKG creation protocol.
func (h *Handler) start(ctx context.Context, start types.Start,
	from mino.Address, out mino.Sender) error {

	dela.Logger.Info().Msgf("%v is starting a DKG", h.me)

	participants := start.GetAddresses()

	if len(participants) != len(start.GetPublicKeys()) {
		return xerrors.Errorf(
			"there should be as many participants as "+
				"pubKey: %d != %d", len(start.GetAddresses()),
			len(start.GetPublicKeys()),
		)
	}

	// 1. Create the DKG
	d, err := pedersen.NewDistKeyGenerator(suite, h.privKey, start.GetPublicKeys(), start.GetThreshold())
	if err != nil {
		return xerrors.Errorf("failed to create new DKG: %v", err)
	}
	h.dkg = d

	// 2. Send my Deals to the other nodes
	err = h.sendDeals(ctx, out, participants)
	if err != nil {
		return xerrors.Errorf("failed to send deals: %v", err)
	}

	// 3. Process the incoming deals
	err = h.receiveDeals(ctx, participants, from, out)
	if err != nil {
		return xerrors.Errorf("failed to receive deals: %v", err)
	}

	h.startRes.SetParticipants(participants)
	h.startRes.SetPublicKeys(start.GetPublicKeys())
	h.startRes.SetThreshold(start.GetThreshold())

	// 4. Certify
	err = h.certify(ctx)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	// 5. Announce the DKG public key
	err = h.announceDkgPublicKey(true, out, from)
	if err != nil {
		return xerrors.Errorf("failed to announce dkg public key: %v", err)
	}

	return nil
}

func (h *Handler) sendDeals(ctx context.Context, out mino.Sender,
	participants []mino.Address) error {

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

		dela.Logger.Trace().Msgf("%s sent deal %d", h.me, i)
		errch := out.Send(dealMsg, participants[i])

		// this should be further improved by using a worker pool,
		// as opposed to an unbounded number of goroutines,
		// but for the time being is okay-ish. -- 2022/08/09
		go func(errs <-chan error, idx int) {
			err := <-errs
			if err != nil {
				dela.Logger.Warn().Msgf(
					"got an error while sending deal %v: %v", i, err)
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
			dela.Logger.Trace().Msgf("%s sent deal %v", h.me, idx)
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

func (h *Handler) receiveDeals(ctx context.Context, participants []mino.Address,
	from mino.Address, out mino.Sender) error {
	dela.Logger.Trace().Msgf("%v is handling deals from other nodes", h.me)

	numReceivedDeals := 0
	for numReceivedDeals < len(participants)-1 {
		select {
		case df := <-h.deals:
			err := h.handleDeal(df.deal, df.from, participants, out)
			if err != nil {
				dela.Logger.Warn().Msgf("%s failed to handle received deal from %s: %v",
					h.me, from, err)
				return xerrors.Errorf("failed to handle received deal: %v", err)
			}

			dela.Logger.Trace().Msgf("%s handled deal #%v from %s",
				h.me, numReceivedDeals, df.from)
			numReceivedDeals++

		case <-ctx.Done():
			dela.Logger.Error().Msgf("%s timed out while receiving deals", h.me)
			return xerrors.Errorf("timed out while receiving deals")
		}
	}

	dela.Logger.Debug().Msgf("%v received all the expected deals", h.me)

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (h *Handler) handleDeal(msg *types.Deal, from mino.Address,
	participants []mino.Address, out mino.Sender) error {
	dela.Logger.Trace().Msgf("%v processing deal from %v\n", h.me, from)

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
		return xerrors.Errorf("failed to process deal from %s: %v", h.me, err)
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

	for _, addr := range participants {
		if addr.Equal(h.me) {
			continue
		}

		dela.Logger.Trace().Msgf("%v sending response to %v ", h.me, addr)

		// this should be further improved by using a worker pool,
		// as opposed to a strictly sequential send,
		// but for the time being is okay-ish. -- 2022/08/09
		errs := out.Send(resp, addr)
		err = <-errs
		if err != nil {
			dela.Logger.Warn().Msgf("got an error while sending response: %v", err)
			return xerrors.Errorf("failed to send response to '%s': %v", addr, err)
		}

	}

	return nil
}

func (h *Handler) certify(ctx context.Context) error {
	dela.Logger.Trace().Msgf("%v is certifying dkg", h.me)

	for !h.dkg.Certified() {
		select {
		case rf, ok := <-h.responses:
			if !ok {
				return xerrors.Errorf("certification aborted: channel closed")
			}

			dela.Logger.Trace().Msgf(
				"%s about to handle response from %s",
				h.me, rf.from,
			)

			msg := rf.response
			response := &pedersen.Response{
				Index: msg.GetIndex(),
				Response: &vss.Response{
					SessionID: msg.GetResponse().GetSessionID(),
					Index:     msg.GetResponse().GetIndex(),
					Status:    msg.GetResponse().GetStatus(),
					Signature: msg.GetResponse().GetSignature(),
				},
			}

			_, err := h.dkg.ProcessResponse(response)
			if err != nil {
				dela.Logger.Warn().Msgf("%s failed to process response: %v", h.me, err)
			} else {
				dela.Logger.Trace().Msgf("%s handled response from %s",
					h.me, rf.from)
			}

		case <-ctx.Done():
			dela.Logger.Error().Msgf("%s timed out while receiving responses", h.me)
			return xerrors.Errorf("timed out while receiving responses")
		}
	}

	h.responses = make(chan responseFrom, 1000)
	h.dealsResharing = make(chan dealFromResharing, 1000)

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

// handles the resharing request - acts differently for the new and old and common nodes
func (h *Handler) handleReshare(out mino.Sender,
	from mino.Address, msg types.ResharingRequest, done chan struct{}) error {

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
		err := h.reshare(ctx, msg, from, out)
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
	from mino.Address, out mino.Sender) error {
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

	// by default the node is not common. later we check
	isCommonNode := false

	// if the node is in the old committee, it should do the following steps
	if isOldNode {
		addrsOld := h.startRes.GetParticipants()

		// this variable is true if the node is common between the old and the new committee
		isCommonNode = isInSlice(h.me, addrsNew) && isInSlice(h.me, addrsOld)

		// 1. update mydkg for resharing
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

	// if the node is a new node or is a common node it should do the next steps
	if isNewNode || isCommonNode {
		// 1. Process the incoming deals
		err := h.receiveDealsResharing(ctx, isCommonNode, resharingRequest, from, out)
		if err != nil {
			return xerrors.Errorf("failed to receive deals: %v", err)
		}

		// save the specifications of the new committee in the handler state
		h.startRes.SetParticipants(resharingRequest.GetAddrsNew())
		h.startRes.SetPublicKeys(resharingRequest.GetPubkeysNew())
		h.startRes.SetThreshold(resharingRequest.TNew)
	}

	// 4. Certify
	// all the nodes should certify
	err := h.certify(ctx)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	// 5. Announce the DKG public key
	// all the old, new and common nodes would announce the public key in this way the initiator can make sure the
	//resharing was completely successful
	err = h.announceDkgPublicKey(isCommonNode, out, from)
	if err != nil {
		return xerrors.Errorf("failed to announce dkg public key: %v", err)
	}

	h.startRes.FinishReshare()

	return nil
}

// similar to sendDeals except that it creates dealResharing which has more data
// than Deal only the old nodes call this function
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
		dealResharingMsg := types.NewDealResharing(dealMsg, publicCoeff)

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

// similar to receiveDeals except that it receives the dealResharing only the
// new or common nodes call this function
func (h *Handler) receiveDealsResharing(ctx context.Context, isCommonNode bool, resharingRequest types.ResharingRequest, from mino.Address, out mino.Sender) error {
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
		select {

		case df := <-h.dealsResharing:
			// the new nodes that are not part of the old committee should
			// create a dkg after receiving their first deal
			if !isCommonNode && numReceivedDeals == 0 {

				c := &pedersen.Config{
					Suite:        suite,
					Longterm:     h.privKey,
					OldNodes:     resharingRequest.GetPubkeysOld(),
					NewNodes:     resharingRequest.GetPubkeysNew(),
					PublicCoeffs: df.dealResharing.GetPublicCoeffs(),
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

			deal := df.dealResharing.GetDeal()

			err := h.handleDeal(&deal, df.from, addrsAll, out)
			if err != nil {
				dela.Logger.Warn().Msgf("%s failed to handle received deal from %s: %v",
					h.me, from, err)
				return xerrors.Errorf("failed to handle received deal: %v", err)
			}

			dela.Logger.Trace().Msgf("%s handled deal from %s", h.me,
				df.from)

			numReceivedDeals++

		case <-ctx.Done():
			dela.Logger.Error().Msgf("%s timed out while receiving deals", h.me)
			return xerrors.Errorf("timed out while receiving deals")
		}
	}

	dela.Logger.Debug().Msgf("%v received all the expected deals", h.me)

	return nil
}

///////// auxiliary functions

// gets an address and a slice of addresses and returns true if that address is
// in the slice.
//  this function is called for cheking whether an old committee member is in the new commmitte as well or not
func isInSlice(addr mino.Address, addrs []mino.Address) bool {

	for _, other := range addrs {
		if addr.Equal(other) {
			return true
		}
	}

	return false
}

// gets the list of the old committee members and new committee members and
// returns the union
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
