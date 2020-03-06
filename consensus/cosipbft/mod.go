// Package cosipbft implements the Consensus interface by using the Collective
// Signing PBFT algorithm defined in the ByzCoin paper. TODO: link
package cosipbft

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Consensus is the implementation of the interface.
// TODO: support asynchronous calls
type Consensus struct {
	cosi    cosi.CollectiveSigning
	mino    mino.Mino
	rpc     mino.RPC
	links   []forwardLink
	factory *ChainFactory
}

// NewCoSiPBFT returns a new instance.
func NewCoSiPBFT(mino mino.Mino, cosi cosi.CollectiveSigning) *Consensus {
	c := &Consensus{
		mino:    mino,
		cosi:    cosi,
		factory: NewChainFactory(cosi.GetVerifier()),
	}

	return c
}

// GetChainFactory returns the chain factory.
func (c *Consensus) GetChainFactory() consensus.ChainFactory {
	return NewChainFactory(c.cosi.GetVerifier())
}

// GetChain return the seals from an index to another.
func (c *Consensus) GetChain(from, to uint64) consensus.Chain {
	// TODO: persistent storage for links
	chain := chain{links: c.links}

	return chain
}

// Listen is a blocking function that makes the consensus available on the
// node.
func (c *Consensus) Listen(v consensus.Validator) error {
	if v == nil {
		return xerrors.New("a validator is required for the consensus")
	}

	err := c.cosi.Listen(handler{Consensus: c, validator: v})
	if err != nil {
		return xerrors.Errorf("couldn't listen: %v", err)
	}

	c.rpc, err = c.mino.MakeRPC("cosipbft", rpcHandler{Consensus: c})
	if err != nil {
		return xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	return nil
}

// Propose takes the proposal and send it to the participants of the consensus.
// It returns nil if the consensus is reached and that the participant are
// committed to it, otherwise it returns the refusal reason.
func (c *Consensus) Propose(p consensus.Proposal, addrs ...mino.Identity) error {
	packed, err := p.Pack()
	if err != nil {
		return encoding.NewEncodingError("proposal", err)
	}

	prepareReq := &Prepare{}
	prepareReq.Proposal, err = ptypes.MarshalAny(packed)
	if err != nil {
		return encoding.NewAnyEncodingError(packed, err)
	}

	cosigners := make([]cosi.Cosigner, len(addrs))
	for i, addr := range addrs {
		cosigners[i] = addr.(cosi.Cosigner)
	}

	// 1. Prepare phase.
	sig, err := c.cosi.Sign(prepareReq, cosigners...)
	if err != nil {
		return xerrors.Errorf("couldn't sign the proposal: %v", err)
	}

	forwardLink := forwardLink{
		from:    nil,
		to:      p.GetHash(),
		prepare: sig,
	}

	flpacked, err := forwardLink.Pack()
	if err != nil {
		return encoding.NewEncodingError("forward link", err)
	}

	commitReq := &Commit{ForwardLink: flpacked.(*ForwardLinkProto)}

	// 2. Commit phase.
	sig, err = c.cosi.Sign(commitReq, cosigners...)
	if err != nil {
		return xerrors.Errorf("couldn't sign the commit: %v", err)
	}

	forwardLink.commit = sig

	// 3. Propagate the final commit signature.
	packed, err = forwardLink.Pack()
	if err != nil {
		return encoding.NewEncodingError("forward link", err)
	}

	tmp := make([]*mino.Address, len(addrs))
	for i, addr := range addrs {
		tmp[i] = addr.Address()
	}

	propagateReq := &Propagate{ForwardLink: packed.(*ForwardLinkProto)}
	resps, errs := c.rpc.Call(propagateReq, tmp...)
	select {
	case <-resps:
	case err := <-errs:
		return xerrors.Errorf("couldn't propagate the link: %v", err)
	}

	return nil
}

func (c *Consensus) verifyPrepareMessage(proposal consensus.Proposal) (forwardLink, error) {
	// TODO: verify correct index, etc etc..
	return forwardLink{}, nil
}

func (c *Consensus) verifyCommitMessage(forwardLink forwardLink) error {
	// TODO: verify prepare signature, integrity of the commit
	// TODO: commit and store.
	return nil
}

type handler struct {
	*Consensus
	validator consensus.Validator
}

func (h handler) Hash(in proto.Message) ([]byte, error) {
	switch msg := in.(type) {
	case *Prepare:
		// The proposal first need to be validated by the caller of the module
		// to insure the generic data is valid.
		// TODO: this should lock during the event propagation to insure atomic
		// operations.
		proposal, err := h.validator.Validate(nil, msg)
		if err != nil {
			return nil, xerrors.Errorf("couldn't validate the proposal: %v", err)
		}

		// Then the consensus layer is verified.
		forwardLink, err := h.verifyPrepareMessage(proposal)
		if err != nil {
			return nil, xerrors.Errorf("invalid proposal: %v", err)
		}

		// Finally, if the proposal is correct, the hash that will be signed
		// by cosi is returned.
		return forwardLink.hash, nil
	case *Commit:
		forwardLink, err := h.factory.decodeLink(msg.GetForwardLink())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode the forward link: %v", err)
		}

		err = h.verifyCommitMessage(*forwardLink)
		if err != nil {
			return nil, xerrors.Errorf("invalid commit: %v", err)
		}

		buffer, err := forwardLink.prepare.MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal the signature: %v", err)
		}

		return buffer, nil
	default:
		return nil, xerrors.New("unknown type of message")
	}
}

type rpcHandler struct {
	*Consensus
	mino.UnsupportedHandler
}

func (h rpcHandler) Process(req proto.Message) (proto.Message, error) {
	msg, ok := req.(*Propagate)
	if !ok {
		return nil, xerrors.New("message type not supported")
	}

	forwardLink, err := h.factory.decodeLink(msg.GetForwardLink())
	if err != nil {
		return nil, xerrors.Errorf("couldn't not decode: %v", err)
	}

	h.links = append(h.links, *forwardLink)

	return nil, nil
}
