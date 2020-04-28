package cosipbft

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Digest is an alias for the bytes type for hash.
type Digest = []byte

// forwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
type forwardLink struct {
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

// Pack returns the protobuf message of the forward link.
func (fl forwardLink) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &ForwardLinkProto{
		From: fl.from,
		To:   fl.to,
	}

	var err error
	pb.ChangeSet, err = fl.packChangeSet(enc)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack changeset: %v", err)
	}

	if fl.prepare != nil {
		pb.Prepare, err = enc.PackAny(fl.prepare)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack prepare signature: %v", err)
		}
	}

	if fl.commit != nil {
		pb.Commit, err = enc.PackAny(fl.commit)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack commit signature: %v", err)
		}
	}

	return pb, nil
}

func (fl forwardLink) packChangeSet(enc encoding.ProtoMarshaler) (*ChangeSet, error) {
	pb := &ChangeSet{
		Leader: fl.changeset.Leader,
		Remove: fl.changeset.Remove,
	}

	var err error
	pb.Add, err = fl.packPlayers(fl.changeset.Add, enc)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack players: %v", err)
	}

	return pb, nil
}

func (fl forwardLink) packPlayers(in []viewchange.Player,
	enc encoding.ProtoMarshaler) ([]*Player, error) {

	if len(in) == 0 {
		// Save some bytes in the message with a nil array.
		return nil, nil
	}

	players := make([]*Player, len(in))
	for i, player := range in {
		addr, err := player.Address.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		pubkey, err := enc.PackAny(player.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack public key: %v", err)
		}

		players[i] = &Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}
	return players, nil
}

func (fl forwardLink) computeHash(h hash.Hash, enc encoding.ProtoMarshaler) ([]byte, error) {
	_, err := h.Write(fl.from)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write 'from': %v", err)
	}
	_, err = h.Write(fl.to)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write 'to': %v", err)
	}

	for _, player := range fl.changeset.Add {
		addr, err := player.Address.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		pubkey, err := player.PublicKey.MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		_, err = h.Write(append(addr, pubkey...))
		if err != nil {
			return nil, xerrors.Errorf("couldn't write player: %v", err)
		}
	}

	buffer := make([]byte, 4+(4*len(fl.changeset.Remove)))
	binary.LittleEndian.PutUint32(buffer, fl.changeset.Leader)

	for i, index := range fl.changeset.Remove {
		binary.LittleEndian.PutUint32(buffer[(i+1)*4:], index)
	}

	_, err = h.Write(buffer)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write integers: %v", err)
	}

	return h.Sum(nil), nil
}

type forwardLinkChain struct {
	links []forwardLink
}

// GetLastHash implements consensus.Chain. It returns the last hash of the
// chain.
func (c forwardLinkChain) GetLastHash() []byte {
	if len(c.links) == 0 {
		return nil
	}

	return c.links[len(c.links)-1].to
}

// Pack returs the protobuf message for the chain.
func (c forwardLinkChain) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &ChainProto{
		Links: make([]*ForwardLinkProto, len(c.links)),
	}

	for i, link := range c.links {
		packed, err := enc.Pack(link)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack forward link: %v", err)
		}

		pb.Links[i] = packed.(*ForwardLinkProto)
	}

	return pb, nil
}

type sha256Factory struct{}

func (f sha256Factory) New() hash.Hash {
	return sha256.New()
}

type unsecureChainFactory struct {
	addrFactory      mino.AddressFactory
	pubkeyFactory    crypto.PublicKeyFactory
	signatureFactory crypto.SignatureFactory
	verifierFactory  crypto.VerifierFactory
	hashFactory      crypto.HashFactory
	encoder          encoding.ProtoMarshaler
}

func newUnsecureChainFactory(cosi cosi.CollectiveSigning, m mino.Mino) *unsecureChainFactory {
	return &unsecureChainFactory{
		addrFactory:      m.GetAddressFactory(),
		pubkeyFactory:    cosi.GetPublicKeyFactory(),
		signatureFactory: cosi.GetSignatureFactory(),
		verifierFactory:  cosi.GetVerifierFactory(),
		hashFactory:      sha256Factory{},
		encoder:          encoding.NewProtoEncoder(),
	}
}

func (f *unsecureChainFactory) decodeForwardLink(pb proto.Message) (forwardLink, error) {
	var fl forwardLink
	var src *ForwardLinkProto
	switch msg := pb.(type) {
	case *any.Any:
		src = &ForwardLinkProto{}

		err := f.encoder.UnmarshalAny(msg, src)
		if err != nil {
			return fl, xerrors.Errorf("couldn't unmarshal forward link: %v", err)
		}
	case *ForwardLinkProto:
		src = msg
	default:
		return fl, xerrors.Errorf("unknown message type: %T", msg)
	}

	fl = forwardLink{
		from: src.GetFrom(),
		to:   src.GetTo(),
	}

	var err error
	fl.changeset, err = f.decodeChangeSet(src.GetChangeSet())
	if err != nil {
		return fl, xerrors.Errorf("couldn't decode changeset: %v", err)
	}

	if src.GetPrepare() != nil {
		sig, err := f.signatureFactory.FromProto(src.GetPrepare())
		if err != nil {
			return fl, xerrors.Errorf("couldn't decode prepare signature: %v", err)
		}

		fl.prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err := f.signatureFactory.FromProto(src.GetCommit())
		if err != nil {
			return fl, xerrors.Errorf("couldn't decode commit signature: %v", err)
		}

		fl.commit = sig
	}

	hash, err := fl.computeHash(f.hashFactory.New(), f.encoder)
	if err != nil {
		return fl, xerrors.Errorf("couldn't hash the forward link: %v", err)
	}

	fl.hash = hash

	return fl, nil
}

func (f *unsecureChainFactory) decodeChangeSet(in *ChangeSet) (viewchange.ChangeSet, error) {
	changeset := viewchange.ChangeSet{
		Leader: in.GetLeader(),
		Remove: in.GetRemove(),
	}

	var err error
	changeset.Add, err = f.decodeAdd(in.GetAdd())
	if err != nil {
		return changeset, xerrors.Errorf("couldn't decode add: %v", err)
	}

	return changeset, nil
}

func (f *unsecureChainFactory) decodeAdd(in []*Player) ([]viewchange.Player, error) {
	if len(in) == 0 {
		return nil, nil
	}

	players := make([]viewchange.Player, len(in))
	for i, player := range in {
		addr := f.addrFactory.FromText(player.GetAddress())
		pubkey, err := f.pubkeyFactory.FromProto(player.GetPublicKey())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode public key: %v", err)
		}

		players[i] = viewchange.Player{
			Address:   addr,
			PublicKey: pubkey,
		}
	}
	return players, nil
}

func (f *unsecureChainFactory) FromProto(pb proto.Message) (consensus.Chain, error) {
	var msg *ChainProto
	switch in := pb.(type) {
	case *any.Any:
		msg = &ChainProto{}

		err := f.encoder.UnmarshalAny(in, msg)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
		}
	case *ChainProto:
		msg = in
	default:
		return nil, xerrors.Errorf("message type not supported: %T", in)
	}

	links := make([]forwardLink, len(msg.GetLinks()))
	for i, plink := range msg.GetLinks() {
		link, err := f.decodeForwardLink(plink)
		if err != nil {
			return nil, err
		}

		links[i] = link
	}

	return forwardLinkChain{links: links}, nil
}

// chainFactory is an implementation of the chainFactory interface
// for forward links.
type chainFactory struct {
	*unsecureChainFactory
	authority viewchange.EvolvableAuthority
}

// newChainFactory returns a new instance of a seal factory that will create
// forward links for appropriate protobuf messages and return an error
// otherwise.
func newChainFactory(cosi cosi.CollectiveSigning, m mino.Mino,
	authority viewchange.EvolvableAuthority) *chainFactory {

	return &chainFactory{
		unsecureChainFactory: newUnsecureChainFactory(cosi, m),
		authority:            authority,
	}
}

// FromProto returns a chain from a protobuf message.
func (f *chainFactory) FromProto(pb proto.Message) (consensus.Chain, error) {
	chain, err := f.unsecureChainFactory.FromProto(pb)
	if err != nil {
		return nil, err
	}

	err = f.verify(chain.(forwardLinkChain))
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the chain: %v", err)
	}

	return chain, nil
}

func (f *chainFactory) verify(chain forwardLinkChain) error {
	if len(chain.links) == 0 {
		return xerrors.New("chain is empty")
	}

	authority := f.authority

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
