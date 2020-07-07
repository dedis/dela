// Package cosipbft implements the Consensus interface by using the Collective
// Signing PBFT algorithm defined in the ByzCoin paper. TODO: link
package cosipbft

import (
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/cosipbft/types"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const (
	rpcName = "cosipbft"
)

// Consensus is the implementation of the consensus.Consensus interface.
type Consensus struct {
	logger      zerolog.Logger
	storage     types.Storage
	cosi        cosi.CollectiveSigning
	mino        mino.Mino
	queue       types.Queue
	hashFactory crypto.HashFactory
	viewchange  viewchange.ViewChange
}

// NewCoSiPBFT returns a new instance.
func NewCoSiPBFT(mino mino.Mino, cosi cosi.CollectiveSigning, vc viewchange.ViewChange) *Consensus {
	c := &Consensus{
		logger:      dela.Logger,
		storage:     types.NewInMemoryStorage(),
		mino:        mino,
		cosi:        cosi,
		queue:       types.NewQueue(cosi.GetVerifierFactory()),
		hashFactory: crypto.NewSha256Factory(),
		viewchange:  vc,
	}

	return c
}

// GetChainFactory implements consensus.Consensus.
func (c *Consensus) GetChainFactory() consensus.ChainFactory {
	return types.NewChainFactory(
		c.cosi.GetSignatureFactory(),
		c.viewchange.GetChangeSetFactory(),
		c.viewchange,
		c.cosi.GetVerifierFactory())
}

// GetChain returns a valid chain to the given identifier.
func (c *Consensus) GetChain(to types.Digest) (consensus.Chain, error) {
	chain, err := c.storage.ReadChain(to)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the chain: %v", err)
	}

	return chain, nil
}

// Listen is a blocking function that makes the consensus available on the
// node.
func (c *Consensus) Listen(r consensus.Reactor) (consensus.Actor, error) {
	if r == nil {
		return nil, xerrors.New("validator is nil")
	}

	factory := types.NewRequestFactory(
		r,
		c.cosi.GetSigner().GetSignatureFactory(),
		c.cosi.GetSignatureFactory(),
		c.GetChainFactory())

	reactor := reactor{
		Consensus:      c,
		RequestFactory: factory,
		reactor:        r,
	}

	cosiActor, err := c.cosi.Listen(reactor)
	if err != nil {
		return nil, xerrors.Errorf("couldn't listen: %w", err)
	}

	handler := rpcHandler{
		Consensus: c,
		reactor:   r,
	}

	rpc, err := c.mino.MakeRPC(rpcName, handler, factory)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %w", err)
	}

	actor := pbftActor{
		Consensus: c,
		cosiActor: cosiActor,
		reactor:   r,
		rpc:       rpc,
		closing:   make(chan struct{}),
	}

	return actor, nil
}

type pbftActor struct {
	*Consensus
	closing   chan struct{}
	cosiActor cosi.Actor
	rpc       mino.RPC
	reactor   consensus.Reactor
}

// Propose implements consensus.Actor. It takes the proposal and send it to the
// participants of the consensus. It returns nil if the consensus is reached and
// the participant are committed to it, otherwise it returns the refusal reason.
func (a pbftActor) Propose(p serde.Message) error {
	// Wait for the view change module green signal to go through the proposal.
	// If the leader has failed and this node has to take over, we use the
	// inherant property of CoSiPBFT to prove that 2f participants want the view
	// change.
	ok := a.viewchange.Wait()
	if !ok {
		dela.Logger.Trace().Msg("proposal skipped by view change")
		// Not authorized to propose a block as the leader is moving forward so
		// we drop the proposal. The upper layer is responsible for trying again
		// until the leader includes the data.
		return nil
	}

	digest, err := a.reactor.InvokeValidate(a.mino.GetAddress(), p)
	if err != nil {
		return xerrors.Errorf("couldn't validate proposal: %v", err)
	}

	prepareReq, err := a.newPrepareRequest(p, digest)
	if err != nil {
		return xerrors.Errorf("couldn't create prepare request: %v", err)
	}

	authority, err := a.viewchange.GetAuthority(a.storage.Len())
	if err != nil {
		return xerrors.Errorf("couldn't read authority for id %#x: %v", digest, err)
	}

	// 1. Prepare phase: proposal must be validated by the nodes and a
	// collective signature will be created for the forward link hash.
	ctx := context.Background()
	sig, err := a.cosiActor.Sign(ctx, prepareReq, authority)
	if err != nil {
		return xerrors.Errorf("couldn't sign the proposal: %v", err)
	}

	commitReq := types.NewCommit(digest, sig)

	// 2. Commit phase.
	sig, err = a.cosiActor.Sign(ctx, commitReq, authority)
	if err != nil {
		return xerrors.Errorf("couldn't sign the commit: %v", err)
	}

	// 3. Propagate the final commit signature.
	propagateReq := types.NewPropagate(digest, sig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := a.rpc.Call(ctx, propagateReq, authority)
	if err != nil {
		return xerrors.Errorf("call aborted: %v", err)
	}

	for {
		select {
		case <-a.closing:
			// Abort the RPC call.
			cancel()
			return nil
		case resp, more := <-resps:
			if !more {
				return nil
			}

			_, err = resp.GetMessageOrError()
			if err != nil {
				a.logger.Warn().Err(err).Msg("couldn't propagate the link")
			}
		}
	}
}

func (a pbftActor) newPrepareRequest(msg serde.Message, digest []byte) (types.Prepare, error) {
	chain, err := a.storage.ReadChain(nil)
	if err != nil {
		return types.Prepare{}, xerrors.Errorf("couldn't read chain: %v", err)
	}

	// Sign the hash of the proposal to provide a proof the proposal comes from
	// the legitimate leader.
	sig, err := a.cosi.GetSigner().Sign(digest)
	if err != nil {
		return types.Prepare{}, xerrors.Errorf("couldn't sign the request: %v", err)
	}

	req := types.NewPrepare(msg, sig, chain)

	return req, nil
}

// Close implements consensus.Actor. It announces a close event to allow current
// execution to be aborted.
func (a pbftActor) Close() error {
	close(a.closing)
	return nil
}

type reactor struct {
	types.RequestFactory
	*Consensus

	reactor consensus.Reactor
}

func (h reactor) Invoke(addr mino.Address, in serde.Message) (types.Digest, error) {
	switch msg := in.(type) {
	case types.Prepare:
		err := h.storage.StoreChain(msg.GetChain())
		if err != nil {
			return nil, xerrors.Errorf("couldn't store previous chain: %v", err)
		}

		// The proposal first needs to be validated by the caller of the module
		// to insure the generic data is valid.
		digest, err := h.reactor.InvokeValidate(addr, msg.GetMessage())
		if err != nil {
			return nil, xerrors.Errorf("couldn't validate the proposal: %v", err)
		}

		authority, err := h.viewchange.Verify(addr, h.storage.Len())
		if err != nil {
			return nil, xerrors.Errorf("couldn't verify: %v", err)
		}

		from := msg.GetChain().GetTo()

		if len(from) == 0 {
			genesis, err := h.reactor.InvokeGenesis()
			if err != nil {
				return nil, xerrors.Errorf("couldn't get genesis id: %v", err)
			}

			from = genesis
		}

		forwardLink, err := types.NewForwardLink(from, digest)
		if err != nil {
			return nil, xerrors.Errorf("couldn't create link: %v", err)
		}

		// The identity of the leader must be insured to comply with the
		// viewchange property. The Signature should be verified with the leader
		// public key.
		pubkey, _ := authority.GetPublicKey(addr)
		if pubkey == nil {
			return nil, xerrors.Errorf("couldn't find public key for <%v>", addr)
		}

		err = pubkey.Verify(digest, msg.GetSignature())
		if err != nil {
			return nil, xerrors.Errorf("couldn't verify signature: %v", err)
		}

		err = h.queue.New(forwardLink, authority)
		if err != nil {
			return nil, xerrors.Errorf("couldn't add to queue: %v", err)
		}

		// Finally, if the proposal is correct, the hash that will be signed
		// by cosi is returned.
		return forwardLink.GetFingerprint(), nil
	case types.Commit:
		err := h.queue.LockProposal(msg.GetTo(), msg.GetPrepare())
		if err != nil {
			return nil, xerrors.Errorf("couldn't update signature: %v", err)
		}

		buffer, err := msg.GetPrepare().MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal the signature: %v", err)
		}

		return buffer, nil
	default:
		return nil, xerrors.Errorf("message type not supported '%T'", msg)
	}
}

type rpcHandler struct {
	*Consensus
	mino.UnsupportedHandler

	reactor consensus.Reactor
}

func (h rpcHandler) Process(req mino.Request) (serde.Message, error) {
	msg, ok := req.Message.(types.Propagate)
	if !ok {
		return nil, xerrors.Errorf("message type not supported '%T'", req.Message)
	}

	// 1. Verify the commit signature to make sure a threshold of nodes have
	// agreed to commit to this proposal (and thus not another one).
	forwardLink, err := h.queue.Finalize(msg.GetTo(), msg.GetCommit())
	if err != nil {
		return nil, xerrors.Errorf("couldn't finalize: %v", err)
	}

	// 2. Get the current authority that might evolve after applying the
	// proposal.
	index := h.storage.Len()
	curr, err := h.viewchange.GetAuthority(index)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get authority: %v", err)
	}

	// 3. Apply the proposal to caller. This should persist any change related
	// to the proposal and the system should move to the next state.
	err = h.reactor.InvokeCommit(forwardLink.GetTo())
	if err != nil {
		return nil, xerrors.Errorf("couldn't commit: %v", err)
	}

	// 4. Retrieve the change set of the authority for this forward link by
	// making a diff of the new authority value. This may be empty.
	next, err := h.viewchange.GetAuthority(index + 1)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get new authority: %v", err)
	}

	final, err := types.NewForwardLink(
		forwardLink.GetFrom(), forwardLink.GetTo(),
		types.WithPrepare(forwardLink.GetPrepareSignature()),
		types.WithCommit(forwardLink.GetCommitSignature()),
		types.WithChangeSet(curr.Diff(next)),
	)

	if err != nil {
		return nil, err
	}

	// TODO: check no more than f = (n-1)/2 changes, probably

	// 5. Persist the link.
	// TODO: what if that fails ?
	err = h.storage.Store(final)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write forward link: %v", err)
	}

	return nil, nil
}
