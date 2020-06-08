// Package cosipbft implements the Consensus interface by using the Collective
// Signing PBFT algorithm defined in the ByzCoin paper. TODO: link
package cosipbft

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	rpcName = "cosipbft"
)

// Consensus is the implementation of the consensus.Consensus interface.
type Consensus struct {
	logger       zerolog.Logger
	storage      Storage
	cosi         cosi.CollectiveSigning
	mino         mino.Mino
	queue        Queue
	encoder      encoding.ProtoMarshaler
	hashFactory  crypto.HashFactory
	chainFactory consensus.ChainFactory
	viewchange   viewchange.ViewChange
}

// NewCoSiPBFT returns a new instance.
func NewCoSiPBFT(mino mino.Mino, cosi cosi.CollectiveSigning, vc viewchange.ViewChange) *Consensus {
	c := &Consensus{
		logger:       dela.Logger,
		storage:      newInMemoryStorage(),
		mino:         mino,
		cosi:         cosi,
		queue:        newQueue(cosi),
		encoder:      encoding.NewProtoEncoder(),
		hashFactory:  crypto.NewSha256Factory(),
		chainFactory: newUnsecureChainFactory(cosi, mino),
		viewchange:   vc,
	}

	return c
}

// GetChainFactory returns the chain factory.
func (c *Consensus) GetChainFactory() (consensus.ChainFactory, error) {
	authority, err := c.viewchange.GetGenesis()
	if err != nil {
		return nil, xerrors.Errorf("couldn't get genesis authority: %v", err)
	}

	return newChainFactory(c.cosi, c.mino, authority), nil
}

// GetChain returns a valid chain to the given identifier.
func (c *Consensus) GetChain(to Digest) (consensus.Chain, error) {
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

	cosiActor, err := c.cosi.Listen(handler{Consensus: c, reactor: r})
	if err != nil {
		return nil, xerrors.Errorf("couldn't listen: %w", err)
	}

	rpc, err := c.mino.MakeRPC(rpcName, rpcHandler{Consensus: c, reactor: r})
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
func (a pbftActor) Propose(p proto.Message) error {
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
		return err
	}

	prepareReq, err := a.newPrepareRequest(p, digest)
	if err != nil {
		return xerrors.Errorf("couldn't create prepare request: %v", err)
	}

	authority, err := a.viewchange.GetAuthority()
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

	commitReq := newCommitRequest(digest, sig)

	// 2. Commit phase.
	sig, err = a.cosiActor.Sign(ctx, commitReq, authority)
	if err != nil {
		return xerrors.Errorf("couldn't sign the commit: %v", err)
	}

	// 3. Propagate the final commit signature.
	propagateReq := &PropagateRequest{To: digest}
	propagateReq.Commit, err = a.encoder.PackAny(sig)
	if err != nil {
		return xerrors.Errorf("couldn't pack signature: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, errs := a.rpc.Call(ctx, propagateReq, authority)
	for {
		select {
		case <-a.closing:
			// Abort the RPC call.
			cancel()
			return nil
		case <-resps:
			return nil
		case err := <-errs:
			a.logger.Warn().Err(err).Msg("couldn't propagate the link")
		}
	}
}

func (a pbftActor) newPrepareRequest(msg proto.Message, digest []byte) (Prepare, error) {
	chain, err := a.storage.ReadChain(nil)
	if err != nil {
		return Prepare{}, err
	}

	// Sign the hash of the proposal to provide a proof the proposal comes from
	// the legitimate leader.
	sig, err := a.cosi.GetSigner().Sign(digest)
	if err != nil {
		return Prepare{}, xerrors.Errorf("couldn't sign the request: %v", err)
	}

	req := Prepare{
		message:   msg,
		signature: sig,
		chain:     chain,
	}

	return req, nil
}

// Close implements consensus.Actor. It announces a close event to allow current
// execution to be aborted.
func (a pbftActor) Close() error {
	close(a.closing)
	return nil
}

type handler struct {
	*Consensus

	reactor consensus.Reactor
}

func (h handler) Invoke(addr mino.Address, in proto.Message) (Digest, error) {
	w, ok := in.(*any.Any)
	if ok {
		// TODO: remove after serde migration
		in, _ = h.encoder.UnmarshalDynamicAny(w)
	}

	switch msg := in.(type) {
	case *PrepareRequest:
		chain, err := h.chainFactory.FromProto(msg.Chain)
		if err != nil {
			return nil, err
		}

		err = h.storage.StoreChain(chain)
		if err != nil {
			return nil, xerrors.Errorf("couldn't store previous chain: %v", err)
		}

		proposalpb, err := h.encoder.UnmarshalDynamicAny(msg.GetProposal())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal proposal: %v", err)
		}

		// The proposal first needs to be validated by the caller of the module
		// to insure the generic data is valid.
		digest, err := h.reactor.InvokeValidate(addr, proposalpb)
		if err != nil {
			return nil, xerrors.Errorf("couldn't validate the proposal: %v", err)
		}

		currAuthority, nextAuthority, err := h.viewchange.Verify(addr)
		if err != nil {
			return nil, xerrors.Errorf("couldn't verify: %v", err)
		}

		forwardLink := forwardLink{
			from:      chain.GetTo(),
			to:        digest,
			changeset: currAuthority.Diff(nextAuthority),
		}

		hash, err := forwardLink.computeHash(h.hashFactory.New(), h.encoder)
		if err != nil {
			return nil, xerrors.Errorf("couldn't compute hash: %v", err)
		}

		forwardLink.hash = hash

		// The identity of the leader must be insured to comply with the
		// viewchange property. The Signature should be verified with the leader
		// public key.
		sig, err := h.cosi.GetSigner().GetSignatureFactory().FromProto(msg.GetSignature())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode signature: %v", err)
		}

		pubkey, _ := currAuthority.GetPublicKey(addr)
		if pubkey == nil {
			return nil, xerrors.Errorf("couldn't find public key for <%v>", addr)
		}

		err = pubkey.Verify(digest, sig)
		if err != nil {
			return nil, xerrors.Errorf("couldn't verify signature: %v", err)
		}

		err = h.queue.New(forwardLink, currAuthority)
		if err != nil {
			return nil, xerrors.Errorf("couldn't add to queue: %v", err)
		}

		// Finally, if the proposal is correct, the hash that will be signed
		// by cosi is returned.
		return hash, nil
	case *CommitRequest:
		prepare, err := h.cosi.GetSignatureFactory().FromProto(msg.GetPrepare())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode prepare signature: %v", err)
		}

		err = h.queue.LockProposal(msg.GetTo(), prepare)
		if err != nil {
			return nil, xerrors.Errorf("couldn't update signature: %v", err)
		}

		buffer, err := prepare.MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal the signature: %v", err)
		}

		return buffer, nil
	default:
		return nil, xerrors.Errorf("message type not supported: %T", msg)
	}
}

type rpcHandler struct {
	*Consensus
	mino.UnsupportedHandler

	reactor consensus.Reactor
}

func (h rpcHandler) Process(req mino.Request) (proto.Message, error) {
	msg, ok := req.Message.(*PropagateRequest)
	if !ok {
		return nil, xerrors.Errorf("message type not supported: %T", req.Message)
	}

	commit, err := h.cosi.GetSignatureFactory().FromProto(msg.GetCommit())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode commit signature: %v", err)
	}

	forwardLink, err := h.queue.Finalize(msg.GetTo(), commit)
	if err != nil {
		return nil, xerrors.Errorf("couldn't finalize: %v", err)
	}

	err = h.storage.Store(*forwardLink)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write forward link: %v", err)
	}

	// Apply the proposal to caller.
	err = h.reactor.InvokeCommit(forwardLink.to)
	if err != nil {
		return nil, xerrors.Errorf("couldn't commit: %v", err)
	}

	return nil, nil
}
