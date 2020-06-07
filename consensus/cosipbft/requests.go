package cosipbft

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"golang.org/x/xerrors"
)

// Prepare is the request sent at the beginning of the PBFT protocol.
type Prepare struct {
	message   proto.Message
	signature crypto.Signature
}

// Pack returns the protobuf message, or an error.
func (p Prepare) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	proposal, err := enc.MarshalAny(p.message)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack proposal: %v", err)
	}

	signature, err := enc.PackAny(p.signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack signature: %v", err)
	}

	pb := &PrepareRequest{
		Proposal:  proposal,
		Signature: signature,
	}

	return pb, nil
}

// Commit is the request sent for the last phase of the PBFT.
type Commit struct {
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

// Pack returns the protobuf message representation of a commit, or an error if
// something goes wrong during encoding.
func (c Commit) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &CommitRequest{
		To: c.to,
	}

	var err error
	pb.Prepare, err = enc.PackAny(c.prepare)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack prepare signature: %v", err)
	}

	return pb, nil
}
