// Package cosipbft implements the Consensus interface by using the Collective
// Signing PBFT algorithm defined in the ByzCoin paper. TODO: link
package cosipbft

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi"
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
	storage Storage
	cosi    cosi.CollectiveSigning
	mino    mino.Mino
	rpc     mino.RPC
	factory ChainFactory
	queue   Queue
}

// NewCoSiPBFT returns a new instance.
func NewCoSiPBFT(mino mino.Mino, cosi cosi.CollectiveSigning) *Consensus {
	chainFactory := newChainFactory(cosi.GetVerifier())

	c := &Consensus{
		storage: newInMemoryStorage(),
		mino:    mino,
		cosi:    cosi,
		factory: chainFactory,
		queue: &queue{
			verifier: cosi.GetVerifier(),
			factory:  chainFactory,
		},
	}

	return c
}

// GetChainFactory returns the chain factory.
func (c *Consensus) GetChainFactory() consensus.ChainFactory {
	return c.factory
}

// GetChain returns a valid chain to the given identifier.
func (c *Consensus) GetChain(id Digest) (consensus.Chain, error) {
	stored, err := c.storage.ReadChain(id)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the chain: %v", err)
	}

	chain, err := c.factory.FromProto(&ChainProto{Links: stored})
	if err != nil {
		return nil, encoding.NewDecodingError("chain", err)
	}

	return chain, nil
}

// Listen is a blocking function that makes the consensus available on the
// node.
func (c *Consensus) Listen(v consensus.Validator) error {
	if v == nil {
		return xerrors.New("validator is nil")
	}

	err := c.cosi.Listen(handler{Consensus: c, validator: v})
	if err != nil {
		return xerrors.Errorf("couldn't listen: %w", err)
	}

	c.rpc, err = c.mino.MakeRPC(rpcName, rpcHandler{Consensus: c, validator: v})
	if err != nil {
		return xerrors.Errorf("couldn't create the rpc: %w", err)
	}

	return nil
}

// Propose takes the proposal and send it to the participants of the consensus.
// It returns nil if the consensus is reached and that the participant are
// committed to it, otherwise it returns the refusal reason.
func (c *Consensus) Propose(p consensus.Proposal, nodes ...mino.Node) error {
	packed, err := p.Pack()
	if err != nil {
		return encoding.NewEncodingError("proposal", err)
	}

	prepareReq := &Prepare{}
	prepareReq.Proposal, err = protoenc.MarshalAny(packed)
	if err != nil {
		return encoding.NewAnyEncodingError(packed, err)
	}

	var ok bool
	cosigners := make([]cosi.Cosigner, len(nodes))
	for i, addr := range nodes {
		cosigners[i], ok = addr.(cosi.Cosigner)
		if !ok {
			return xerrors.New("node must implement cosi.Cosigner")
		}
	}

	// 1. Prepare phase: proposal must be validated by the nodes and a
	// collective signature will be created for the forward link hash.
	sig, err := c.cosi.Sign(prepareReq, cosigners...)
	if err != nil {
		return xerrors.Errorf("couldn't sign the proposal: %v", err)
	}

	sigpacked, err := sig.Pack()
	if err != nil {
		return encoding.NewEncodingError("prepare signature", err)
	}

	commitReq := &Commit{To: p.GetHash()}
	commitReq.Prepare, err = protoenc.MarshalAny(sigpacked)
	if err != nil {
		return encoding.NewAnyEncodingError(sigpacked, err)
	}

	// 2. Commit phase.
	sig, err = c.cosi.Sign(commitReq, cosigners...)
	if err != nil {
		return xerrors.Errorf("couldn't sign the commit: %v", err)
	}

	sigpacked, err = sig.Pack()
	if err != nil {
		return encoding.NewEncodingError("commit signature", err)
	}

	// 3. Propagate the final commit signature.
	propagateReq := &Propagate{To: p.GetHash()}
	propagateReq.Commit, err = protoenc.MarshalAny(sigpacked)
	if err != nil {
		return encoding.NewAnyEncodingError(packed, err)
	}

	// TODO: timeout in context ?
	resps, errs := c.rpc.Call(propagateReq, nodes...)
	select {
	case <-resps:
	case err := <-errs:
		return xerrors.Errorf("couldn't propagate the link: %v", err)
	}

	return nil
}

type handler struct {
	*Consensus
	validator consensus.Validator
}

func (h handler) Hash(in proto.Message) (Digest, error) {
	switch msg := in.(type) {
	case *Prepare:
		var da ptypes.DynamicAny
		err := protoenc.UnmarshalAny(msg.GetProposal(), &da)
		if err != nil {
			return nil, encoding.NewAnyDecodingError(&da, err)
		}

		// The proposal first needs to be validated by the caller of the module
		// to insure the generic data is valid.
		//
		// TODO: this should lock during the event propagation to insure atomic
		// operations.
		proposal, prev, err := h.validator.Validate(da.Message)
		if err != nil {
			return nil, xerrors.Errorf("couldn't validate the proposal: %v", err)
		}

		last, err := h.storage.ReadLast()
		if err != nil {
			return nil, xerrors.Errorf("couldn't read last: %v", err)
		}

		if last != nil && !bytes.Equal(last.GetTo(), prev.GetHash()) {
			return nil, xerrors.Errorf("mismatch with previous link: %x != %x",
				last.GetTo(), prev.GetHash())
		}

		forwardLink := forwardLink{
			from: prev.GetHash(),
			to:   proposal.GetHash(),
		}

		err = h.queue.New(proposal, prev)
		if err != nil {
			return nil, xerrors.Errorf("couldn't add to queue: %v", err)
		}

		hash, err := forwardLink.computeHash(h.factory.GetHashFactory().New())
		if err != nil {
			return nil, xerrors.Errorf("couldn't compute hash: %v", err)
		}

		// Finally, if the proposal is correct, the hash that will be signed
		// by cosi is returned.
		return hash, nil
	case *Commit:
		prepare, err := h.factory.DecodeSignature(msg.GetPrepare())
		if err != nil {
			return nil, encoding.NewDecodingError("prepare signature", err)
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

func (h rpcHandler) Process(req proto.Message) (proto.Message, error) {
	msg, ok := req.(*Propagate)
	if !ok {
		return nil, xerrors.Errorf("message type not supported: %T", req)
	}

	commit, err := h.factory.DecodeSignature(msg.GetCommit())
	if err != nil {
		return nil, encoding.NewDecodingError("commit signature", err)
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
