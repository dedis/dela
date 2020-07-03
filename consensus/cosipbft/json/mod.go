package json

import (
	"encoding/json"

	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/cosipbft/types"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterForwardLinkFormat(serde.FormatJSON, linkFormat{})
	types.RegisterChainFormat(serde.FormatJSON, chainFormat{})
	types.RegisterRequestFormat(serde.FormatJSON, requestFormat{})
}

// ForwardLink is the JSON message for a forward link.
type ForwardLink struct {
	From      []byte
	To        []byte
	Prepare   json.RawMessage
	Commit    json.RawMessage
	ChangeSet json.RawMessage
}

// Chain is the JSON message for the forward link chain.
type Chain []ForwardLink

// PrepareRequest is the JSON message to send the prepare request for a new
// proposal.
type PrepareRequest struct {
	Message   json.RawMessage
	Signature json.RawMessage
	Chain     json.RawMessage
}

// CommitRequest is the JSON message that contains the prepare signature to
// commit to a new proposal.
type CommitRequest struct {
	To      []byte
	Prepare json.RawMessage
}

// PropagateRequest is the JSON message sent when a proposal has been accepted
// by the network.
type PropagateRequest struct {
	To     []byte
	Commit json.RawMessage
}

// Request is a wrapper of the request JSON messages. It allows both types to be
// deserialized by the same factory.
type Request struct {
	Prepare   *PrepareRequest   `json:",omitempty"`
	Commit    *CommitRequest    `json:",omitempty"`
	Propagate *PropagateRequest `json:",omitempty"`
}

// LinkFormat is the engine to encode and decode forward link messages in JSON
// format.
//
// - implements serde.FormatEngine
type linkFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data in JSON
// format for the link message if appropriate, otherwise an error.
func (f linkFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	link, ok := msg.(types.ForwardLink)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	m, err := f.toJSON(ctx, link)
	if err != nil {
		return nil, err
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the forward link for the
// JSON data if appropriate, otherwise it returns an error.
func (f linkFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := ForwardLink{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal link: %v", err)
	}

	link, err := f.fromJSON(ctx, m)
	if err != nil {
		return nil, err
	}

	return *link, nil
}

func (f linkFormat) toJSON(ctx serde.Context, link types.ForwardLink) (*ForwardLink, error) {
	var changeset json.RawMessage
	var err error
	if link.GetChangeSet() != nil {
		changeset, err = link.GetChangeSet().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize changeset: %v", err)
		}
	}

	prepare, err := link.GetPrepareSignature().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize prepare signature: %v", err)
	}

	commit, err := link.GetCommitSignature().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize commit signature: %v", err)
	}

	m := &ForwardLink{
		From:      link.GetFrom(),
		To:        link.GetTo(),
		Prepare:   prepare,
		Commit:    commit,
		ChangeSet: changeset,
	}

	return m, nil
}

func (f linkFormat) fromJSON(ctx serde.Context, m ForwardLink) (*types.ForwardLink, error) {
	factory := ctx.GetFactory(types.CoSigKeyFac{})

	sf, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid signature factory of type '%T'", factory)
	}

	prepare, err := sf.SignatureOf(ctx, m.Prepare)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize prepare: %v", err)
	}

	commit, err := sf.SignatureOf(ctx, m.Commit)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize commit: %v", err)
	}

	factory = ctx.GetFactory(types.ChangeSetKeyFac{})

	csf, ok := factory.(viewchange.ChangeSetFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid change set factory of type '%T'", factory)
	}

	changeset, err := csf.ChangeSetOf(ctx, m.ChangeSet)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize change set: %v", err)
	}

	opts := []types.ForwardLinkOption{
		types.WithPrepare(prepare),
		types.WithCommit(commit),
		types.WithChangeSet(changeset),
	}

	link, err := types.NewForwardLink(m.From, m.To, opts...)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create forward link: %v", err)
	}

	return &link, nil
}

// ChainFormat is the engine to encode and decode chain messages in JSON format.
//
// - implements serde.FormatEngine
type chainFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data in JSON
// format for the chain message if appropriate, otherwise an error.
func (f chainFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	chain, ok := msg.(types.Chain)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	linkFormat := linkFormat{}

	links := make([]ForwardLink, chain.Len())
	for i, link := range chain.GetLinks() {
		raw, err := linkFormat.toJSON(ctx, link)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize link: %v", err)
		}

		links[i] = *raw
	}

	m := Chain(links)

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the chain for the JSON
// data if appropriate, otherwise it returns an error.
func (f chainFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Chain{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize chain: %v", err)
	}

	linkFormat := linkFormat{}

	links := make([]types.ForwardLink, len(m))
	for i, link := range m {
		link, err := linkFormat.fromJSON(ctx, link)
		if err != nil {
			return nil, err
		}

		links[i] = *link
	}

	return types.NewChain(links...), nil
}

// RequestFormat is the engine to encode and decode request messages in JSON
// format.
//
// - implements serde.FormatEngine
type requestFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data in JSON
// format for the request message if appropriate, otherwise an error.
func (f requestFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var req Request

	switch in := msg.(type) {
	case types.Prepare:
		message, err := in.GetMessage().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize message: %v", err)
		}

		signature, err := in.GetSignature().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize signature: %v", err)
		}

		chain, err := in.GetChain().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize chain: %v", err)
		}

		m := PrepareRequest{
			Message:   message,
			Signature: signature,
			Chain:     chain,
		}

		req = Request{Prepare: &m}
	case types.Commit:
		prepare, err := in.GetPrepare().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize prepare: %v", err)
		}

		m := CommitRequest{
			To:      in.GetTo(),
			Prepare: prepare,
		}

		req = Request{Commit: &m}
	case types.Propagate:
		commit, err := in.GetCommit().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize commit: %v", err)
		}

		m := PropagateRequest{
			To:     in.GetTo(),
			Commit: commit,
		}

		req = Request{Propagate: &m}
	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	data, err := ctx.Marshal(req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the request for the JSON
// data if appropriate, otherwise it returns an error.
func (f requestFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Request{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal request: %v", err)
	}

	if m.Prepare != nil {
		message, err := decodeMessage(ctx, m.Prepare.Message)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
		}

		signature, err := decodeSignature(ctx, m.Prepare.Signature)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
		}

		chain, err := decodeChain(ctx, m.Prepare.Chain)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize chain: %v", err)
		}

		p := types.NewPrepare(message, signature, chain)

		return p, nil
	}

	if m.Commit != nil {
		prepare, err := decodeCoSignature(ctx, m.Commit.Prepare)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize commit: %v", err)
		}

		c := types.NewCommit(m.Commit.To, prepare)

		return c, nil
	}

	if m.Propagate != nil {
		commit, err := decodeCoSignature(ctx, m.Propagate.Commit)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize propagate: %v", err)
		}

		p := types.NewPropagate(m.Propagate.To, commit)

		return p, nil
	}

	return nil, xerrors.New("message is empty")
}

func decodeMessage(ctx serde.Context, data []byte) (serde.Message, error) {
	factory := ctx.GetFactory(types.MsgKeyFac{})

	return factory.Deserialize(ctx, data)
}

func decodeSignature(ctx serde.Context, data []byte) (crypto.Signature, error) {
	factory := ctx.GetFactory(types.SigKeyFac{})

	sf, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	return sf.SignatureOf(ctx, data)
}

func decodeChain(ctx serde.Context, data []byte) (consensus.Chain, error) {
	factory := ctx.GetFactory(types.ChainKeyFac{})

	cf, ok := factory.(consensus.ChainFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	return cf.ChainOf(ctx, data)
}

func decodeCoSignature(ctx serde.Context, data []byte) (crypto.Signature, error) {
	factory := ctx.GetFactory(types.CoSigKeyFac{})

	sf, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	return sf.SignatureOf(ctx, data)
}
