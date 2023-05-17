package pedersen

import (
	"context"
	"crypto/sha256"
	"strconv"
	"sync"

	"github.com/dedis/debugtools/channel"
	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
	"golang.org/x/xerrors"
)

// constant used in the logs
const newState = "new state"
const badState = "bad state: %v"
const failedState = "failed to switch state: %v"

type nodeType byte

// enumeration of the node type
const (
	unknownNode nodeType = iota
	oldNode
	commonNode
	newNode
)

// dkgService specifies all we need from the kyber library
type dkgService interface {
	Deals() (map[int]*pedersen.Deal, error)
	ProcessResponse(resp *pedersen.Response) (*pedersen.Justification, error)
	Certified() bool
	DistKeyShare() (*pedersen.DistKeyShare, error)
	ProcessDeal(dd *pedersen.Deal) (*pedersen.Response, error)
}

// dkgInstance specify what a stream handler needs from a service that handles
// dkg operations.
type dkgInstance interface {
	isRunning() bool
	handleMessage(ctx context.Context, msg serde.Message, from mino.Address, out mino.Sender) error
	getState() *state
}

// newInstance returns a new initialized dkg handler
func newInstance(log zerolog.Logger, me mino.Address, privKey kyber.Scalar) *instance {
	return &instance{
		running:   true,
		deals:     channel.WithExpiration[types.Deal](200),
		responses: channel.WithExpiration[types.Response](100000),
		reshares:  channel.WithExpiration[types.Reshare](200),

		log:     log,
		me:      me,
		privKey: privKey,

		startRes: &state{
			dkgState: initial,
		},
	}
}

// instance implements the default dkgInstance
//
// - implements dkgInstance
type instance struct {
	sync.Mutex

	running bool

	deals     channel.Timed[types.Deal]
	responses channel.Timed[types.Response]
	reshares  channel.Timed[types.Reshare]

	dkg dkgService
	log zerolog.Logger
	me  mino.Address

	privShare *share.PriShare
	privKey   kyber.Scalar

	startRes *state
}

// isRunning implements dkgInstance. It tells if an instance of DKG is already
// running or not.
func (s *instance) isRunning() bool {
	s.Lock()
	defer s.Unlock()

	return s.running
}

// getState implements dkgInstance. It returns the internal state.
func (s *instance) getState() *state {
	return s.startRes
}

// handleMessage implements dkgInstance. It handles the DKG messages.
func (s *instance) handleMessage(ctx context.Context, msg serde.Message, from mino.Address, out mino.Sender) error {
	// We expect a Start message or a decrypt request at first, but we might
	// receive other messages in the meantime, like a Deal.
	switch msg := msg.(type) {

	case types.Start:
		go func() {
			err := s.start(ctx, msg, s.deals, s.responses, from, out)
			if err != nil {
				s.log.Err(err).Msg("failed to start")
			}

			s.Lock()
			s.running = false
			s.Unlock()
		}()

	case types.StartResharing:
		go func() {
			err := s.reshare(ctx, out, from, msg, s.reshares, s.responses)
			if err != nil {
				s.log.Err(err).Msg("failed to handle resharing")
			}

			s.Lock()
			s.running = false
			s.Unlock()
		}()

	case types.Deal:
		err := s.startRes.checkState(initial, sharing, certified, resharing)
		if err != nil {
			return xerrors.Errorf(badState, err)
		}

		s.deals.Send(msg)

	case types.Reshare:
		err := s.startRes.checkState(initial, certified, resharing)
		if err != nil {
			return xerrors.Errorf(badState, err)
		}

		s.reshares.Send(msg)

	case types.Response:
		err := s.startRes.checkState(initial, sharing, certified, resharing)
		if err != nil {
			return xerrors.Errorf(badState, err)
		}

		s.responses.Send(msg)

	case types.DecryptRequest:
		err := s.startRes.checkState(certified)
		if err != nil {
			return xerrors.Errorf(badState, err)
		}

		return s.handleDecrypt(out, msg, from)

	case types.VerifiableDecryptRequest:
		err := s.startRes.checkState(certified)
		if err != nil {
			return xerrors.Errorf(badState, err)
		}

		return s.handleVerifiableDecrypt(out, msg, from)

	case types.ReencryptRequest:
		err := s.startRes.checkState(certified)
		if err != nil {
			return xerrors.Errorf(badState, err)
		}

		return s.handleReencryptRequest(out, msg, from)

	default:
		return xerrors.Errorf("expected Start message, decrypt request or "+
			"Deal as first message, got: %T", msg)
	}

	return nil
}

// start is called when the node has received its start message. Note that we
// might have already received some deals from other nodes in the meantime. The
// function handles the DKG creation protocol.
func (s *instance) start(ctx context.Context, start types.Start, deals channel.Timed[types.Deal],
	resps channel.Timed[types.Response], from mino.Address, out mino.Sender) error {

	err := s.startRes.switchState(sharing)
	if err != nil {
		return xerrors.Errorf(failedState, err)
	}

	if len(start.GetAddresses()) != len(start.GetPublicKeys()) {
		return xerrors.Errorf("there should be as many participants as "+
			"pubKey: %d != %d", len(start.GetAddresses()), len(start.GetPublicKeys()))
	}

	// create the DKG
	t := start.GetThreshold()
	d, err := pedersen.NewDistKeyGenerator(suite, s.privKey, start.GetPublicKeys(), t)
	if err != nil {
		return xerrors.Errorf("failed to create new DKG: %v", err)
	}

	s.dkg = d

	s.startRes.init(start.GetAddresses(), start.GetPublicKeys(), start.GetThreshold())

	err = s.doDKG(ctx, deals, resps, out, from)
	if err != nil {
		xerrors.Errorf("something went wrong during DKG: %v", err)
	}

	return nil
}

// doDKG calls the subsequent DKG steps
func (s *instance) doDKG(ctx context.Context, deals channel.Timed[types.Deal],
	resps channel.Timed[types.Response], out mino.Sender, from mino.Address) error {

	defer func() {
		s.Lock()
		s.running = false
		s.Unlock()
	}()

	s.log.Info().Str("action", "deal").Msg(newState)
	err := s.deal(ctx, out)
	if err != nil {
		return xerrors.Errorf("failed to deal: %v", err)
	}

	s.log.Info().Str("action", "respond").Msg(newState)
	err = s.respond(ctx, deals, out)
	if err != nil {
		return xerrors.Errorf("failed to respond: %v", err)
	}

	s.log.Info().Str("action", "certify").Msg(newState)
	numResps := (len(s.startRes.getParticipants()) - 1) * (len(s.startRes.getParticipants()) - 1)
	err = s.certify(ctx, resps, numResps)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	err = s.startRes.switchState(certified)
	if err != nil {
		return xerrors.Errorf(failedState, err)
	}

	s.log.Info().Str("action", "finalize").Msg(newState)
	err = s.finalize(ctx, from, out)
	if err != nil {
		return xerrors.Errorf("failed to finalize: %v", err)
	}

	s.log.Info().Str("action", "done").Msg(newState)

	return nil
}

func (s *instance) deal(ctx context.Context, out mino.Sender) error {
	// Send my Deals to the other nodes. Note that we take an optimistic
	// approach and expect nodes to always accept messages. If not, the protocol
	// can hang forever.

	deals, err := s.dkg.Deals()
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

		to := s.startRes.getParticipants()[i]

		s.log.Trace().Str("to", to.String()).Msg("send deal")

		errs := out.Send(dealMsg, to)

		// this can be blocking if the recipient is not receiving message
		select {
		case err = <-errs:
			if err != nil {
				s.log.Err(err).Str("to", to.String()).Msg("failed to send deal")
			}
		case <-ctx.Done():
			return xerrors.Errorf("context done: %v", ctx.Err())
		}
	}

	return nil
}

func (s *instance) respond(ctx context.Context, deals channel.Timed[types.Deal], out mino.Sender) error {
	numReceivedDeals := 0

	for numReceivedDeals < len(s.startRes.getParticipants())-1 {
		deal, err := deals.NonBlockingReceiveWithContext(ctx)
		if err != nil {
			return xerrors.Errorf("context done: %v", err)
		}

		err = s.handleDeal(ctx, deal, out, s.startRes.getParticipants())
		if err != nil {
			return xerrors.Errorf("failed to handle received deal: %v", err)
		}

		numReceivedDeals++

		s.log.Trace().Str("total", strconv.Itoa(numReceivedDeals)).Msg("deal received")
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
func (s *instance) certify(ctx context.Context, resps channel.Timed[types.Response], expected int) error {

	responsesReceived := 0

	for responsesReceived < expected {
		msg, err := resps.NonBlockingReceiveWithContext(ctx)
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

		_, err = s.dkg.ProcessResponse(&resp)
		if err != nil {
			return xerrors.Errorf("failed to process response: %v", err)
		}

		responsesReceived++

		s.log.Trace().Int("total", responsesReceived).Msg("response processed")
	}

	if !s.dkg.Certified() {
		return xerrors.New("node is not certified")
	}

	return nil
}

// finalize saves the result and announces it to the orchestrator.
func (s *instance) finalize(ctx context.Context, from mino.Address, out mino.Sender) error {
	// Send back the public DKG key
	distKey, err := s.dkg.DistKeyShare()
	if err != nil {
		return xerrors.Errorf("failed to get distr key: %v", err)
	}

	// Update the state before sending the acknowledgement to the orchestrator,
	// so that it can process decrypt requests right away.
	s.startRes.setDistKey(distKey.Public())

	s.Lock()
	s.privShare = distKey.PriShare()
	s.Unlock()

	done := types.NewStartDone(distKey.Public())

	select {
	case err = <-out.Send(done, from):
		if err != nil {
			return xerrors.Errorf("got an error while sending pub key: %v", err)
		}
	case <-ctx.Done():
		s.log.Warn().Msgf("context done (this might be expected): %v", ctx.Err())
	}

	return nil
}

// handleDeal process the Deal and send the responses to the other nodes.
func (s *instance) handleDeal(ctx context.Context, msg types.Deal,
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

	response, err := s.dkg.ProcessDeal(deal)
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
		if addr.Equal(s.me) {
			continue
		}

		s.log.Trace().Str("to", addr.String()).Uint32("dealer", response.Index).
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

func (s *instance) finalizeReshare(ctx context.Context, nt nodeType, out mino.Sender, from mino.Address) error {
	// Send back the public DKG key
	publicKey := s.startRes.getDistKey()

	// if the node is new or a common node it should update its state
	if publicKey == nil || nt == commonNode {
		distrKey, err := s.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("failed to get distr key: %v", err)
		}

		publicKey = distrKey.Public()

		// Update the state before sending to acknowledgement to the
		// orchestrator, so that it can process decrypt requests right away.
		s.startRes.setDistKey(distrKey.Public())
		s.Lock()
		s.privShare = distrKey.PriShare()
		s.Unlock()
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
		s.log.Warn().Msgf("context done (this might be expected): %v", ctx.Err())
	}

	dela.Logger.Info().Msgf("%s announced the DKG public key", s.me)

	return nil
}

// reshare handles the resharing request. Acts differently for the new
// and old and common nodes
func (s *instance) reshare(ctx context.Context, out mino.Sender,
	from mino.Address, msg types.StartResharing, reshares channel.Timed[types.Reshare], resps channel.Timed[types.Response]) error {

	err := s.startRes.switchState(resharing)
	if err != nil {
		return xerrors.Errorf(failedState, err)
	}

	addrsNew := msg.GetAddrsNew()

	if len(addrsNew) != len(msg.GetPubkeysNew()) {
		return xerrors.Errorf("there should be as many participants as pubKey: %d != %d",
			len(addrsNew), len(msg.GetPubkeysNew()))
	}

	err = s.doReshare(ctx, msg, from, out, reshares, resps)
	if err != nil {
		xerrors.Errorf("failed to reshare: %v", err)
	}

	return nil
}

// doReshare is called when the node has received its reshare message. Note that
// we might have already received some deals from other nodes in the meantime.
// The function handles the DKG resharing protocol.
func (s *instance) doReshare(ctx context.Context, start types.StartResharing,
	from mino.Address, out mino.Sender, reshares channel.Timed[types.Reshare], resps channel.Timed[types.Response]) error {

	s.log.Info().Msgf("resharing with %v", start.GetAddrsNew())

	var expectedResponses int
	addrsOld := s.startRes.getParticipants()
	addrsNew := start.GetAddrsNew()

	nt := newNode
	if s.startRes.getDistKey() != nil {
		if isInSlice(s.me, addrsNew) && isInSlice(s.me, addrsOld) {
			nt = commonNode
		} else {
			nt = oldNode
		}
	}

	switch nt {
	case oldNode:
		// Update local DKG for resharing
		share, err := s.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("old node failed to create: %v", err)
		}

		s.log.Trace().Msgf("old node: %v", s.startRes.getPublicKeys())

		c := &pedersen.Config{
			Suite:        suite,
			Longterm:     s.privKey,
			OldNodes:     s.startRes.getPublicKeys(),
			NewNodes:     start.GetPubkeysNew(),
			Share:        share,
			Threshold:    start.GetTNew(),
			OldThreshold: s.startRes.getThreshold(),
		}

		d, err := pedersen.NewDistKeyHandler(c)
		if err != nil {
			return xerrors.Errorf("old node failed to compute the new dkg: %v", err)
		}

		s.dkg = d

		// Send my Deals to the new and common nodes
		err = s.sendDealsResharing(ctx, out, addrsNew, share.Commits)
		if err != nil {
			return xerrors.Errorf("old node failed to send deals: %v", err)
		}

		expectedResponses = (start.GetTNew()) * len(s.startRes.getParticipants())

	case commonNode:
		// Update local DKG for resharing
		share, err := s.dkg.DistKeyShare()
		if err != nil {
			return xerrors.Errorf("common node failed to create: %v", err)
		}

		s.log.Trace().Msgf("old node: %v", s.startRes.getPublicKeys())

		c := &pedersen.Config{
			Suite:        suite,
			Longterm:     s.privKey,
			OldNodes:     s.startRes.getPublicKeys(),
			NewNodes:     start.GetPubkeysNew(),
			Share:        share,
			Threshold:    start.GetTNew(),
			OldThreshold: s.startRes.getThreshold(),
		}

		d, err := pedersen.NewDistKeyHandler(c)
		if err != nil {
			return xerrors.Errorf("common node failed to compute the new dkg: %v", err)
		}

		s.dkg = d

		// Send my Deals to the new and common nodes
		err = s.sendDealsResharing(ctx, out, addrsNew, share.Commits)
		if err != nil {
			return xerrors.Errorf("common node failed to send deals: %v", err)
		}

		// Process the incoming deals
		err = s.receiveDealsResharing(ctx, nt, start, out, reshares)
		if err != nil {
			return xerrors.Errorf("common node failed to receive deals: %v", err)
		}

		// Save the specifications of the new committee in the handler state
		s.startRes.init(start.GetAddrsNew(), start.GetPubkeysNew(), start.GetTNew())

		expectedResponses = (start.GetTNew() - 1) * len(addrsOld)

	case newNode:
		// Process the incoming deals
		err := s.receiveDealsResharing(ctx, nt, start, out, reshares)
		if err != nil {
			return xerrors.Errorf("new node failed to receive deals: %v", err)
		}

		// Save the specifications of the new committee in the handler state
		s.startRes.init(start.GetAddrsNew(), start.GetPubkeysNew(), start.GetTNew())

		expectedResponses = (start.GetTNew() - 1) * start.GetTOld()
	}

	// All nodes should certify.
	err := s.certify(ctx, resps, expectedResponses)
	if err != nil {
		return xerrors.Errorf("failed to certify: %v", err)
	}

	err = s.startRes.switchState(certified)
	if err != nil {
		return xerrors.Errorf(failedState, err)
	}

	// Announce the DKG public key All the old, new and common nodes would
	// announce the public key. In this way the initiator can make sure the
	// resharing was completely successful.
	err = s.finalizeReshare(ctx, nt, out, from)
	if err != nil {
		return xerrors.Errorf("failed to announce dkg public key: %v", err)
	}

	return nil
}

// sendDealsResharing is similar to sendDeals except that it creates
// dealResharing which has more data than Deal. Only the old nodes call this
// function.
func (s *instance) sendDealsResharing(ctx context.Context, out mino.Sender,
	participants []mino.Address, publicCoeff []kyber.Point) error {

	s.log.Trace().Msgf("%v is generating its deals", s.me)

	deals, err := s.dkg.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

	s.log.Trace().Msgf("%s is sending its deals", s.me)

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

		s.log.Trace().Msgf("%s sent dealResharing %d", s.me, i)

		select {
		case err := <-out.Send(dealResharingMsg, participants[i]):
			if err != nil {
				return xerrors.Errorf("failed to send resharing deal: %v", err)
			}
		case <-ctx.Done():
			return xerrors.Errorf("context done: %v", ctx.Err())
		}
	}

	s.log.Debug().Msgf("%s sent all its deals", s.me)

	return nil
}

// receiveDealsResharing is similar to receiveDeals except that it receives the
// dealResharing. Only the new or common nodes call this function
func (s *instance) receiveDealsResharing(ctx context.Context, nt nodeType,
	resharingRequest types.StartResharing, out mino.Sender, reshares channel.Timed[types.Reshare]) error {

	s.log.Trace().Msgf("%v is handling deals from other nodes", s.me)

	addrsNew := resharingRequest.GetAddrsNew()
	var addrsOld []mino.Address

	numReceivedDeals := 0
	expectedDeals := 0

	// by default we read the old addresses from the state
	addrsOld = s.startRes.getParticipants()

	// the new nodes don't have the old addresses in their state
	if nt == newNode {
		addrsOld = resharingRequest.GetAddrsOld()
	}

	expectedDeals = len(addrsOld)

	// we find the union of the old and new address sets to avoid from sending a
	// message to the common nodes multiple times
	addrsAll := union(addrsNew, addrsOld)

	for numReceivedDeals < expectedDeals {
		reshare, err := reshares.NonBlockingReceiveWithContext(ctx)
		if err != nil {
			return xerrors.Errorf("context done: %v", err)
		}

		if nt == newNode && numReceivedDeals == 0 {

			c := &pedersen.Config{
				Suite:        suite,
				Longterm:     s.privKey,
				OldNodes:     resharingRequest.GetPubkeysOld(),
				NewNodes:     resharingRequest.GetPubkeysNew(),
				PublicCoeffs: reshare.GetPublicCoeffs(),
				Threshold:    resharingRequest.GetTNew(),
				OldThreshold: resharingRequest.GetTOld(),
			}

			newDkg, err := pedersen.NewDistKeyHandler(c)
			if err != nil {
				return xerrors.Errorf("failed to create the new dkg for %s: %v", s.me, err)
			}

			s.dkg = newDkg
		}

		deal := reshare.GetDeal()

		err = s.handleDeal(ctx, deal, out, addrsAll)
		if err != nil {
			return xerrors.Errorf("failed to handle received deal: %v", err)
		}

		numReceivedDeals++
	}

	s.log.Debug().Msgf("%v received all the expected deals", s.me)

	return nil
}

func (s *instance) handleDecrypt(out mino.Sender, msg types.DecryptRequest,
	from mino.Address) error {

	if !s.startRes.Done() {
		return xerrors.Errorf("you must first initialize DKG. Did you call setup() first?")
	}

	S := suite.Point().Mul(s.privShare.V, msg.K)

	partial := suite.Point().Sub(msg.C, S)
	decryptReply := types.NewDecryptReply(int64(s.privShare.I), partial)

	errs := out.Send(decryptReply, from)
	err := <-errs
	if err != nil {
		return xerrors.Errorf("got an error while sending the decrypt reply: %v", err)
	}

	return nil
}

func (s *instance) handleReencryptRequest(out mino.Sender, msg types.ReencryptRequest,
	from mino.Address) error {

	if !s.startRes.Done() {
		return xerrors.Errorf("you must first initialize DKG. Did you call setup() first?")
	}

	ui := s.getUI(msg.U, msg.PubK)

	// Calculating proofs
	si := suite.Scalar().Pick(suite.RandomStream())
	uiHat := suite.Point().Mul(si, suite.Point().Add(msg.U, msg.PubK))
	hiHat := suite.Point().Mul(si, nil)
	hash := sha256.New()
	ui.V.MarshalTo(hash)
	uiHat.MarshalTo(hash)
	hiHat.MarshalTo(hash)
	ei := suite.Scalar().SetBytes(hash.Sum(nil))
	fi := suite.Scalar().Add(si, suite.Scalar().Mul(ei, s.privShare.V))

	response := types.NewReencryptReply(ui, ei, fi)

	errs := out.Send(response, from)
	err := <-errs
	if err != nil {
		return xerrors.Errorf("got an error while sending the reencrypt reply: %v", err)
	}

	return nil
}

func (s *instance) getUI(U, pubk kyber.Point) *share.PubShare {
	v := suite.Point().Mul(s.privShare.V, U)
	v.Add(v, suite.Point().Mul(s.privShare.V, pubk))
	return &share.PubShare{
		I: s.privShare.I,
		V: v,
	}
}

func (s *instance) handleVerifiableDecrypt(out mino.Sender,
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
				sp, err := verifiableDecryption(j.ct, s.privShare.V, s.privShare.I)
				if err != nil {
					s.log.Err(err).Msg("verifiable decryption failed")
				}

				shareAndProofs[j.index] = *sp
			}

		}()
	}

	wgBatchReply.Wait()

	s.log.Info().Msg("sending back verifiable decrypt reply")

	verifiableDecryptReply := types.NewVerifiableDecryptReply(shareAndProofs)

	errs := out.Send(verifiableDecryptReply, from)
	err := <-errs
	if err != nil {
		return xerrors.Errorf("failed to send verifiable decrypt: %v", err)
	}

	return nil
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
