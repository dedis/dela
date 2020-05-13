// Package cosipbft implements the Consensus interface by using the Collective
// Signing PBFT algorithm defined in the ByzCoin paper. TODO: link
package cosipbft

import (
	"bytes"
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/consensus/viewchange/constant"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	rpcName = "cosipbft"
)

// Consensus is the implementation of the consensus.Consensus interface.
type Consensus struct {
	storage      Storage
	cosi         cosi.CollectiveSigning
	mino         mino.Mino
	queue        Queue
	encoder      encoding.ProtoMarshaler
	hashFactory  crypto.HashFactory
	chainFactory consensus.ChainFactory
	governance   viewchange.Governance

	// ViewChange can be personalized after instantiation.
	ViewChange viewchange.ViewChange
}

// NewCoSiPBFT returns a new instance.
func NewCoSiPBFT(mino mino.Mino, cosi cosi.CollectiveSigning, gov viewchange.Governance) *Consensus {
	c := &Consensus{
		storage:      newInMemoryStorage(),
		mino:         mino,
		cosi:         cosi,
		queue:        newQueue(cosi),
		encoder:      encoding.NewProtoEncoder(),
		hashFactory:  crypto.NewSha256Factory(),
		chainFactory: newUnsecureChainFactory(cosi, mino),
		governance:   gov,
		ViewChange:   constant.NewViewChange(mino.GetAddress()),
	}

	return c
}

// GetChainFactory returns the chain factory.
func (c *Consensus) GetChainFactory() (consensus.ChainFactory, error) {
	authority, err := c.governance.GetAuthority(0)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get genesis authority: %v", err)
	}

	return newChainFactory(c.cosi, c.mino, authority), nil
}

// GetChain returns a valid chain to the given identifier.
func (c *Consensus) GetChain(to Digest) (consensus.Chain, error) {
	stored, err := c.storage.ReadChain(to)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the chain: %v", err)
	}

	// The chain stored has already been verified so we can skip that step.
	chain, err := c.chainFactory.FromProto(&ChainProto{Links: stored})
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode chain: %v", err)
	}

	return chain, nil
}

// Listen is a blocking function that makes the consensus available on the
// node.
func (c *Consensus) Listen(v consensus.Validator) (consensus.Actor, error) {
	if v == nil {
		return nil, xerrors.New("validator is nil")
	}

	actor := pbftActor{
		Consensus: c,
		closing:   make(chan struct{}),
	}

	var err error
	actor.cosiActor, err = c.cosi.Listen(handler{Consensus: c, validator: v})
	if err != nil {
		return nil, xerrors.Errorf("couldn't listen: %w", err)
	}

	actor.rpc, err = c.mino.MakeRPC(rpcName, rpcHandler{Consensus: c, validator: v})
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %w", err)
	}

	return actor, nil
}

// Store implements consensus.Consensus. It stores the chain by following the
// local one and completes it. It returns an error if a link is inconsistent
// with the local storage.
func (c *Consensus) Store(in consensus.Chain) error {
	chain, ok := in.(forwardLinkChain)
	if !ok {
		return xerrors.Errorf("invalid message type '%T' != '%T'", in, chain)
	}

	last, err := c.storage.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read latest chain: %v", err)
	}

	store := false
	for _, link := range chain.links {
		store = store || last == nil || bytes.Equal(last.To, link.from[:])

		if store {
			linkpb, err := c.encoder.Pack(link)
			if err != nil {
				return xerrors.Errorf("couldn't pack link: %v", err)
			}

			err = c.storage.Store(linkpb.(*ForwardLinkProto))
			if err != nil {
				return xerrors.Errorf("couldn't store link: %v", err)
			}
		}
	}

	return nil
}

type pbftActor struct {
	*Consensus
	closing   chan struct{}
	cosiActor cosi.Actor
	rpc       mino.RPC
}

// Propose implements consensus.Actor. It takes the proposal and send it to the
// participants of the consensus. It returns nil if the consensus is reached and
// the participant are committed to it, otherwise it returns the refusal reason.
func (a pbftActor) Propose(p consensus.Proposal) error {
	authority, err := a.governance.GetAuthority(p.GetIndex() - 1)
	if err != nil {
		return xerrors.Errorf("couldn't read authority for index %d: %v",
			p.GetIndex()-1, err)
	}

	// Wait for the view change module green signal to go through the proposal.
	// If the leader has failed and this node has to take over, we use the
	// inherant property of CoSiPBFT to prove that 2f participants want the view
	// change.
	leader, ok := a.ViewChange.Wait(p, authority)
	if !ok {
		fabric.Logger.Trace().Msg("proposal skipped by view change")
		// Not authorized to propose a block as the leader is moving forward so
		// we drop the proposal. The upper layer is responsible for trying again
		// until the leader includes the data.
		return nil
	}

	changeset, err := a.governance.GetChangeSet(p)
	if err != nil {
		return xerrors.Errorf("couldn't get change set: %v", err)
	}

	changeset.Leader = leader

	ctx := context.Background()
	prepareReq, err := a.newPrepareRequest(p, changeset)
	if err != nil {
		return xerrors.Errorf("couldn't create prepare request: %v", err)
	}

	// 1. Prepare phase: proposal must be validated by the nodes and a
	// collective signature will be created for the forward link hash.
	sig, err := a.cosiActor.Sign(ctx, prepareReq, authority)
	if err != nil {
		return xerrors.Errorf("couldn't sign the proposal: %v", err)
	}

	commitReq, err := newCommitRequest(p.GetHash(), sig)
	if err != nil {
		return xerrors.Errorf("couldn't create commit request: %w", err)
	}

	// 2. Commit phase.
	sig, err = a.cosiActor.Sign(ctx, commitReq, authority)
	if err != nil {
		return xerrors.Errorf("couldn't sign the commit: %v", err)
	}

	// 3. Propagate the final commit signature.
	propagateReq := &PropagateRequest{To: p.GetHash()}
	propagateReq.Commit, err = a.encoder.PackAny(sig)
	if err != nil {
		return xerrors.Errorf("couldn't pack signature: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, errs := a.rpc.Call(ctx, propagateReq, authority)
	select {
	case <-a.closing:
		// Abort the RPC call.
		cancel()
	case <-resps:
	case err := <-errs:
		return xerrors.Errorf("couldn't propagate the link: %v", err)
	}

	return nil
}

func (a pbftActor) newPrepareRequest(prop consensus.Proposal,
	cs viewchange.ChangeSet) (Prepare, error) {

	req := Prepare{proposal: prop}

	// Sign the hash of the proposal to provide a proof the proposal comes from
	// the legitimate leader.
	sig, err := a.cosi.GetSigner().Sign(prop.GetHash())
	if err != nil {
		return req, xerrors.Errorf("couldn't sign the request: %v", err)
	}

	req.signature = sig

	forwardLink := forwardLink{
		from:      prop.GetPreviousHash(),
		to:        prop.GetHash(),
		changeset: cs,
	}

	digest, err := forwardLink.computeHash(a.hashFactory.New(), a.encoder)
	if err != nil {
		return req, xerrors.Errorf("couldn't compute hash: %v", err)
	}

	req.digest = digest

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
	validator consensus.Validator
}

func (h handler) Hash(addr mino.Address, in proto.Message) (Digest, error) {
	switch msg := in.(type) {
	case *PrepareRequest:
		proposalpb, err := h.encoder.UnmarshalDynamicAny(msg.GetProposal())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal proposal: %v", err)
		}

		// The proposal first needs to be validated by the caller of the module
		// to insure the generic data is valid.
		proposal, err := h.validator.Validate(addr, proposalpb)
		if err != nil {
			return nil, xerrors.Errorf("couldn't validate the proposal: %v", err)
		}

		authority, err := h.governance.GetAuthority(proposal.GetIndex() - 1)
		if err != nil {
			return nil, xerrors.Errorf("couldn't read authority: %v", err)
		}

		last, err := h.storage.ReadLast()
		if err != nil {
			return nil, xerrors.Errorf("couldn't read last: %v", err)
		}

		if last != nil && !bytes.Equal(last.GetTo(), proposal.GetPreviousHash()) {
			return nil, xerrors.Errorf("mismatch with previous link: %x != %x",
				last.GetTo(), proposal.GetPreviousHash())
		}

		changeset, err := h.governance.GetChangeSet(proposal)
		if err != nil {
			return nil, xerrors.Errorf("couldn't get change set: %v", err)
		}

		forwardLink := forwardLink{
			from:      proposal.GetPreviousHash(),
			to:        proposal.GetHash(),
			changeset: changeset,
		}

		leader := h.ViewChange.Verify(proposal, authority)

		// The identity of the leader must be insured to comply with the
		// viewchange property. The Signature should be verified with the leader
		// public key.
		sig, err := h.cosi.GetSigner().GetSignatureFactory().FromProto(msg.GetSignature())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode signature: %v", err)
		}

		iter := authority.PublicKeyIterator()
		iter.Seek(int(leader))
		if iter.HasNext() {
			err := iter.GetNext().Verify(proposal.GetHash(), sig)
			if err != nil {
				return nil, xerrors.Errorf("couldn't verify signature: %v", err)
			}
		} else {
			return nil, xerrors.Errorf("unknown public key at index %d", leader)
		}

		// In case it matches, we keep track of the previous leader for
		// optimization.
		forwardLink.changeset.Leader = leader

		err = h.queue.New(forwardLink, authority)
		if err != nil {
			return nil, xerrors.Errorf("couldn't add to queue: %v", err)
		}

		hash, err := forwardLink.computeHash(h.hashFactory.New(), h.encoder)
		if err != nil {
			return nil, xerrors.Errorf("couldn't compute hash: %v", err)
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

	validator consensus.Validator
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

	err = h.storage.Store(forwardLink)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write forward link: %v", err)
	}

	// Apply the proposal to caller.
	err = h.validator.Commit(forwardLink.GetTo())
	if err != nil {
		return nil, xerrors.Errorf("couldn't commit: %v", err)
	}

	return nil, nil
}
