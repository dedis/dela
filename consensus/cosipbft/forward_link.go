package cosipbft

import (
	"bytes"
	"hash"

	"go.dedis.ch/dela/consensus/cosipbft/json"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Digest is an alias for the bytes type for hash.
type Digest = []byte

// forwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
//
// - implements serde.Message
type forwardLink struct {
	serde.UnimplementedMessage

	hash Digest
	from Digest
	to   Digest
	// prepare is the signature of the combination of From and To to prove that
	// the nodes agreed on a valid forward link between the two blocks.
	prepare crypto.Signature
	// commit is the signature of the Prepare signature to prove that a
	// threshold of the nodes have committed the block.
	commit crypto.Signature
	// changeset announces the changes in the authority for the next proposal.
	changeset viewchange.ChangeSet
}

// Verify makes sure the signatures of the forward link are correct.
func (fl forwardLink) Verify(v crypto.Verifier) error {
	err := v.Verify(fl.hash, fl.prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify prepare signature: %w", err)
	}

	buffer, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %w", err)
	}

	err = v.Verify(buffer, fl.commit)
	if err != nil {
		return xerrors.Errorf("couldn't verify commit signature: %w", err)
	}

	return nil
}

// VisitJSON implements serde.Message. It serializes the forward link in JSON
// format.
func (fl forwardLink) VisitJSON(ser serde.Serializer) (interface{}, error) {
	var changeset []byte
	var err error
	if fl.changeset != nil {
		changeset, err = ser.Serialize(fl.changeset)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize changeset: %v", err)
		}
	}

	prepare, err := ser.Serialize(fl.prepare)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize prepare signature: %v", err)
	}

	commit, err := ser.Serialize(fl.commit)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize commit signature: %v", err)
	}

	m := json.ForwardLink{
		From:      fl.from,
		To:        fl.to,
		Prepare:   prepare,
		Commit:    commit,
		ChangeSet: changeset,
	}

	return m, nil
}

func (fl forwardLink) computeHash(h hash.Hash) ([]byte, error) {
	_, err := h.Write(fl.from)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write 'from': %v", err)
	}
	_, err = h.Write(fl.to)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write 'to': %v", err)
	}

	return h.Sum(nil), nil
}

// ForwardLinkChain is a chain of forward links that can prove the correctness
// of a proposal.
//
// - implements consensus.Chain
// - implements serde.Message
type forwardLinkChain struct {
	serde.UnimplementedMessage

	links []forwardLink
}

// GetLastHash implements consensus.Chain. It returns the last hash of the
// chain.
func (c forwardLinkChain) GetTo() []byte {
	if len(c.links) == 0 {
		return nil
	}

	return c.links[len(c.links)-1].to
}

// VisitJSON implements serde.Message. It serializes the chain in JSON format.
func (c forwardLinkChain) VisitJSON(ser serde.Serializer) (interface{}, error) {
	links := make([]json.ForwardLink, len(c.links))
	for i, link := range c.links {
		packed, err := link.VisitJSON(ser)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize link: %v", err)
		}

		links[i] = packed.(json.ForwardLink)
	}

	return json.Chain(links), nil
}

// chainFactory is an implementation of the chainFactory interface
// for forward links.
//
// - implements serde.Factory
type chainFactory struct {
	serde.UnimplementedFactory

	sigFactory      serde.Factory
	csFactory       serde.Factory
	hashFactory     crypto.HashFactory
	viewchange      viewchange.ViewChange
	verifierFactory crypto.VerifierFactory
}

// newChainFactory returns a new instance of a seal factory that will create
// forward links for appropriate protobuf messages and return an error
// otherwise.
func newChainFactory(cosi cosi.CollectiveSigning, m mino.Mino, vc viewchange.ViewChange) *chainFactory {
	return &chainFactory{
		sigFactory:      cosi.GetSignatureFactory(),
		csFactory:       vc.GetChangeSetFactory(),
		hashFactory:     crypto.NewSha256Factory(),
		viewchange:      vc,
		verifierFactory: cosi.GetVerifierFactory(),
	}
}

// VisitJSON implements serde.Factory. It deserializes the chain in JSON format.
// The integrity of the chain is verified from the genesis authority.
func (f chainFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Chain{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize chain: %v", err)
	}

	links := make([]forwardLink, len(m))
	for i, link := range m {
		var prepare crypto.Signature
		err = in.GetSerializer().Deserialize(link.Prepare, f.sigFactory, &prepare)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize prepare: %v", err)
		}

		var commit crypto.Signature
		err = in.GetSerializer().Deserialize(link.Commit, f.sigFactory, &commit)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize commit: %v", err)
		}

		var changeset viewchange.ChangeSet
		err = in.GetSerializer().Deserialize(link.ChangeSet, f.csFactory, &changeset)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize change set: %v", err)
		}

		links[i] = forwardLink{
			from:      link.From,
			to:        link.To,
			prepare:   prepare,
			commit:    commit,
			changeset: changeset,
		}

		h := f.hashFactory.New()
		hash, err := links[i].computeHash(h)
		if err != nil {
			return nil, xerrors.Errorf("couldn't compute hash: %v", err)
		}

		links[i].hash = hash
	}

	chain := forwardLinkChain{links: links}

	err = f.verify(chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the chain: %v", err)
	}

	return chain, nil
}

func (f chainFactory) verify(chain forwardLinkChain) error {
	if len(chain.links) == 0 {
		return nil
	}

	authority, err := f.viewchange.GetAuthority(0)
	if err != nil {
		// TODO: what to do if no genesis ? (newcomer)
		return nil
	}

	lastIndex := len(chain.links) - 1
	for i, link := range chain.links {
		verifier, err := f.verifierFactory.FromAuthority(authority)
		if err != nil {
			return xerrors.Errorf("couldn't create the verifier: %v", err)
		}

		err = link.Verify(verifier)
		if err != nil {
			return xerrors.Errorf("couldn't verify link %d: %w", i, err)
		}

		if i != lastIndex && !bytes.Equal(link.to, chain.links[i+1].from) {
			return xerrors.Errorf("mismatch forward link '%x' != '%x'",
				link.to, chain.links[i+1].from)
		}

		authority = authority.Apply(link.changeset)
	}

	return nil
}
