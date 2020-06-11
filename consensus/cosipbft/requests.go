package cosipbft

import (
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/cosipbft/json"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Prepare is the request sent at the beginning of the PBFT protocol.
type Prepare struct {
	serde.UnimplementedMessage

	message   serde.Message
	signature crypto.Signature
	chain     consensus.Chain
}

// VisitJSON implements serde.Messsage. It serializes the prepare request
// message in JSON format.
func (p Prepare) VisitJSON(ser serde.Serializer) (interface{}, error) {
	proposal, err := ser.Serialize(p.message)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack proposal: %v", err)
	}

	signature, err := ser.Serialize(p.signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack signature: %v", err)
	}

	chain, err := ser.Serialize(p.chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack chain: %v", err)
	}

	m := json.PrepareRequest{
		Message:   proposal,
		Signature: signature,
		Chain:     chain,
	}

	return json.Request{Prepare: &m}, nil
}

// Commit is the request sent for the last phase of the PBFT.
type Commit struct {
	serde.UnimplementedMessage

	to      Digest
	prepare crypto.Signature
}

func newCommitRequest(to []byte, prepare crypto.Signature) Commit {
	commit := Commit{
		to:      to,
		prepare: prepare,
	}

	return commit
}

// VisitJSON implements serde.Message. It serializes the commit in JSON format.
func (c Commit) VisitJSON(ser serde.Serializer) (interface{}, error) {
	prepare, err := ser.Serialize(c.prepare)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack prepare signature: %v", err)
	}

	m := json.CommitRequest{
		To:      c.to,
		Prepare: prepare,
	}

	return json.Request{Commit: &m}, nil
}

type requestFactory struct {
	serde.UnimplementedFactory

	msgFactory   serde.Factory
	sigFactory   serde.Factory
	cosiFactory  serde.Factory
	chainFactory serde.Factory
}

func (f requestFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Request{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	if m.Prepare != nil {
		var message serde.Message
		err = in.GetSerializer().Deserialize(m.Prepare.Message, f.msgFactory, &message)
		if err != nil {
			return nil, err
		}

		var signature crypto.Signature
		err = in.GetSerializer().Deserialize(m.Prepare.Signature, f.sigFactory, &signature)
		if err != nil {
			return nil, err
		}

		var chain consensus.Chain
		err = in.GetSerializer().Deserialize(m.Prepare.Chain, f.chainFactory, &chain)
		if err != nil {
			return nil, err
		}

		p := Prepare{
			message:   message,
			signature: signature,
			chain:     chain,
		}

		return p, nil
	}

	if m.Commit != nil {
		var prepare crypto.Signature
		err = in.GetSerializer().Deserialize(m.Commit.Prepare, f.cosiFactory, &prepare)
		if err != nil {
			return nil, err
		}

		c := Commit{
			to:      m.Commit.To,
			prepare: prepare,
		}

		return c, nil
	}

	return nil, xerrors.New("request is empty")
}

// Propagate is the final message sent to commit to a new proposal.
type Propagate struct {
	serde.UnimplementedMessage

	to     []byte
	commit crypto.Signature
}

// VisitJSON implements serde.Message.
func (p Propagate) VisitJSON(ser serde.Serializer) (interface{}, error) {
	commit, err := ser.Serialize(p.commit)
	if err != nil {
		return nil, err
	}

	m := json.PropagateRequest{
		To:     p.to,
		Commit: commit,
	}

	return m, nil
}

type propagateFactory struct {
	serde.UnimplementedFactory

	sigFactory serde.Factory
}

func (f propagateFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.PropagateRequest{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	var commit crypto.Signature
	err = in.GetSerializer().Deserialize(m.Commit, f.sigFactory, &commit)
	if err != nil {
		return nil, err
	}

	p := Propagate{
		to:     m.To,
		commit: commit,
	}

	return p, nil
}
