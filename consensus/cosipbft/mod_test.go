package cosipbft

import (
	"bytes"
	"context"
	fmt "fmt"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Player{},
		&ChangeSet{},
		&ForwardLinkProto{},
		&ChainProto{},
		&PrepareRequest{},
		&CommitRequest{},
		&PropagateRequest{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestConsensus_Basic(t *testing.T) {
	prop := &fakeProposal{hash: []byte{0xbb}, previous: []byte{0xaa}}
	cons, actors, authority := makeConsensus(t, 3, prop)

	initial := authority.Take(mino.RangeFilter(0, 2)).(fake.CollectiveAuthority)
	for _, c := range cons {
		c.governance = fakeGovernance{authority: initial}
	}

	// 1. Send a fake proposal with the initial authority.
	err := actors[0].Propose(prop)
	require.NoError(t, err)

	// 2. Send another fake proposal but with a changeset to add the missing
	// player.
	changeset := viewchange.ChangeSet{Add: []viewchange.Player{{
		Address:   cons[2].mino.GetAddress(),
		PublicKey: authority.GetSigner(2).GetPublicKey(),
	}}}
	for _, c := range cons {
		c.governance = fakeGovernance{authority: initial, changeset: changeset}
	}
	prop.hash = []byte{0xcc}
	prop.previous = []byte{0xbb}
	err = actors[0].Propose(prop)
	require.NoError(t, err)

	// 3. Send another fake proposal but with a changeset to remove the player.
	changeset = viewchange.ChangeSet{Remove: []uint32{2}}
	for _, c := range cons {
		c.governance = fakeGovernance{authority: authority, changeset: changeset}
	}
	prop.hash = []byte{0xdd}
	prop.previous = []byte{0xcc}
	err = actors[0].Propose(prop)
	require.NoError(t, err)

	// 4. Send a final fake proposal with the initial authority.
	for _, c := range cons {
		c.governance = fakeGovernance{authority: initial}
	}
	prop.hash = []byte{0xee}
	prop.previous = []byte{0xdd}
	err = actors[0].Propose(prop)
	require.NoError(t, err)

	chain, err := cons[0].GetChain(prop.GetHash())
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 4)

	chainpb, err := chain.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	cons[0].governance = fakeGovernance{authority: initial}
	factory, err := cons[0].GetChainFactory()
	require.NoError(t, err)

	// Make sure the chain can be verified with the roster changes.
	chain2, err := factory.FromProto(chainpb)
	require.NoError(t, err)
	require.Equal(t, chain, chain2)
}

func TestConsensus_GetChainFactory(t *testing.T) {
	cons := &Consensus{
		mino:       fake.Mino{},
		cosi:       &fakeCosi{},
		governance: fakeGovernance{},
	}

	factory, err := cons.GetChainFactory()
	require.NoError(t, err)
	require.NotNil(t, factory)

	cons.governance = fakeGovernance{err: xerrors.New("oops")}
	_, err = cons.GetChainFactory()
	require.EqualError(t, err, "couldn't get genesis authority: oops")
}

func TestConsensus_GetChain(t *testing.T) {
	cons := &Consensus{
		storage:      newInMemoryStorage(),
		chainFactory: newUnsecureChainFactory(&fakeCosi{}, fake.Mino{}),
	}

	err := cons.storage.Store(&ForwardLinkProto{To: []byte{0xaa}})
	require.NoError(t, err)

	chain, err := cons.GetChain([]byte{0xaa})
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 1)

	cons.chainFactory = badChainFactory{}
	_, err = cons.GetChain([]byte{0xaa})
	require.EqualError(t, err, "couldn't decode chain: oops")

	cons.storage = fakeStorage{}
	_, err = cons.GetChain([]byte{})
	require.EqualError(t, err, "couldn't read the chain: oops")
}

func TestConsensus_Listen(t *testing.T) {
	fakeCosi := &fakeCosi{}
	fakeMino := &fakeMino{}
	cons := &Consensus{
		cosi: fakeCosi,
		mino: fakeMino,
	}

	actor, err := cons.Listen(fakeValidator{})
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.IsType(t, handler{}, fakeCosi.handler)
	require.IsType(t, rpcHandler{}, fakeMino.h)
	require.Equal(t, rpcName, fakeMino.name)

	_, err = cons.Listen(nil)
	require.EqualError(t, err, "validator is nil")

	fakeCosi.err = xerrors.New("cosi error")
	_, err = cons.Listen(fakeValidator{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, fakeCosi.err))

	fakeCosi.err = nil
	fakeMino.err = xerrors.New("rpc error")
	_, err = cons.Listen(fakeValidator{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, fakeMino.err))
}

func TestConsensus_Store(t *testing.T) {
	call := &fake.Call{}
	cons := &Consensus{
		encoder: encoding.NewProtoEncoder(),
		storage: newInMemoryStorage(),
		queue:   fakeQueue{call: call},
	}

	links := []forwardLink{
		{to: Digest{0x01}},
		{from: Digest{0x01}},
	}

	err := cons.Store(forwardLinkChain{links: links})
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())

	err = cons.Store(fakeChain{})
	require.EqualError(t, err,
		"invalid message type 'cosipbft.fakeChain' != 'cosipbft.forwardLinkChain'")

	cons.storage = fakeStorage{}
	err = cons.Store(forwardLinkChain{links: links})
	require.EqualError(t, err, "couldn't read latest chain: oops")

	cons.storage = newInMemoryStorage()
	cons.encoder = fake.BadPackEncoder{}
	err = cons.Store(forwardLinkChain{links: links})
	require.EqualError(t, err, "couldn't pack link: fake error")

	cons.encoder = encoding.NewProtoEncoder()
	links[0].to = Digest{0x02}
	err = cons.Store(forwardLinkChain{links: links})
	require.EqualError(t, err,
		"couldn't store link: mismatch forward link '02' != '01'")
}

func TestActor_Propose(t *testing.T) {
	rpc := &fakeRPC{}
	cosiActor := &fakeCosiActor{}
	actor := &pbftActor{
		Consensus: &Consensus{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: sha256Factory{},
			governance:  fakeGovernance{},
			ViewChange:  fakeViewChange{},
			cosi:        &fakeCosi{},
		},
		closing:   make(chan struct{}),
		rpc:       rpc,
		cosiActor: cosiActor,
	}

	actor.ViewChange = fakeViewChange{denied: true}
	err := actor.Propose(fakeProposal{})
	require.NoError(t, err)

	actor.ViewChange = fakeViewChange{denied: false, leader: 2}
	err = actor.Propose(fakeProposal{hash: []byte{0xaa}})
	require.NoError(t, err)
	require.Len(t, cosiActor.calls, 2)

	prepare := cosiActor.calls[0]["message"].(Prepare)
	require.NotNil(t, prepare)

	commit := cosiActor.calls[1]["message"].(Commit)
	require.NotNil(t, commit)
	require.Equal(t, []byte{0xaa}, commit.to)

	require.Len(t, rpc.calls, 1)
	propagate := rpc.calls[0]["message"].(*PropagateRequest)
	require.NotNil(t, propagate)
	require.Equal(t, []byte{0xaa}, propagate.GetTo())
	checkSignatureValue(t, propagate.GetCommit())

	require.NoError(t, actor.Close())
	err = actor.Propose(fakeProposal{})
	require.NoError(t, err)
}

func TestActor_Failures_Propose(t *testing.T) {
	actor := &pbftActor{
		Consensus: &Consensus{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: sha256Factory{},
			governance:  fakeGovernance{},
			ViewChange:  fakeViewChange{},
			cosi:        &fakeCosi{},
		},
	}

	actor.governance = fakeGovernance{err: xerrors.New("oops")}
	err := actor.Propose(fakeProposal{})
	require.EqualError(t, err, "couldn't read authority for index 0: oops")

	actor.governance = fakeGovernance{errChangeSet: xerrors.New("oops")}
	err = actor.Propose(fakeProposal{})
	require.EqualError(t, err, "couldn't get change set: oops")

	actor.governance = fakeGovernance{}
	actor.cosi = &fakeCosi{signer: fake.NewBadSigner()}
	err = actor.Propose(fakeProposal{})
	require.EqualError(t, err,
		"couldn't create prepare request: couldn't sign the request: fake error")

	actor.cosi = &fakeCosi{}
	actor.cosiActor = &fakeCosiActor{err: xerrors.New("oops")}
	err = actor.Propose(fakeProposal{})
	require.EqualError(t, err, "couldn't sign the proposal: oops")

	actor.cosiActor = &fakeCosiActor{err: xerrors.New("oops"), delay: 1}
	err = actor.Propose(fakeProposal{})
	require.EqualError(t, err, "couldn't sign the commit: oops")

	actor.cosiActor = &fakeCosiActor{}
	actor.encoder = fake.BadPackAnyEncoder{ProtoEncoder: encoding.NewProtoEncoder()}
	err = actor.Propose(fakeProposal{})
	require.EqualError(t, err, "couldn't pack signature: fake error")

	buffer := new(bytes.Buffer)
	actor.encoder = encoding.NewProtoEncoder()
	actor.rpc = &fakeRPC{err: xerrors.New("oops")}
	actor.logger = zerolog.New(buffer)
	err = actor.Propose(fakeProposal{})
	require.NoError(t, err)
	require.Equal(t,
		"{\"level\":\"warn\",\"error\":\"oops\",\"message\":\"couldn't propagate the link\"}\n",
		buffer.String())
}

func TestActor_Close(t *testing.T) {
	actor := &pbftActor{
		closing: make(chan struct{}),
	}

	require.NoError(t, actor.Close())
	_, ok := <-actor.closing
	require.False(t, ok)
}

func TestHandler_Prepare_Hash(t *testing.T) {
	cons := &Consensus{
		storage:     newInMemoryStorage(),
		queue:       &queue{cosi: &fakeCosi{}},
		hashFactory: crypto.NewSha256Factory(),
		encoder:     encoding.NewProtoEncoder(),
		governance:  fakeGovernance{authority: fake.NewAuthority(3, fake.NewSigner)},
		ViewChange:  fakeViewChange{leader: 2},
		cosi:        &fakeCosi{},
	}
	h := handler{
		validator: fakeValidator{proposal: fakeProposal{}},
		Consensus: cons,
	}

	_, err := h.Invoke(nil, &empty.Empty{})
	require.EqualError(t, err, "message type not supported: *empty.Empty")

	empty, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	buffer, err := h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.NoError(t, err)
	require.NotEmpty(t, buffer)

	cons.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = h.Invoke(nil, &PrepareRequest{})
	require.EqualError(t, err, "couldn't unmarshal proposal: fake error")

	cons.encoder = encoding.NewProtoEncoder()
	h.validator = fakeValidator{err: xerrors.New("oops")}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't validate the proposal: oops")

	h.validator = fakeValidator{proposal: fakeProposal{previous: []byte{0xbb}}}
	h.governance = fakeGovernance{err: xerrors.New("oops")}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't read authority: oops")

	h.governance = fakeGovernance{}
	cons.storage = fakeStorage{}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't read last: oops")

	cons.storage = newInMemoryStorage()
	cons.storage.Store(&ForwardLinkProto{To: []byte{0xaa}})
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "mismatch with previous link: aa != bb")

	cons.storage = newInMemoryStorage()
	cons.governance = fakeGovernance{errChangeSet: xerrors.New("oops")}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't get change set: oops")

	cons.governance = fakeGovernance{authority: fake.NewAuthority(3, func() crypto.AggregateSigner {
		return fake.NewSignerWithPublicKey(fake.NewBadPublicKey())
	})}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't verify signature: fake error")

	cons.governance = fakeGovernance{}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "unknown public key at index 2")

	cons.governance = fakeGovernance{authority: fake.NewAuthority(3, fake.NewSigner)}
	cons.cosi = &fakeCosi{signer: fake.NewSignerWithSignatureFactory(fake.NewBadSignatureFactory())}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't decode signature: fake error")

	cons.cosi = &fakeCosi{}
	cons.queue = &queue{locked: true}
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't add to queue: queue is locked")

	cons.queue = &queue{cosi: &fakeCosi{}}
	cons.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = h.Invoke(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err,
		"couldn't compute hash: couldn't write 'from': fake error")
}

func TestHandler_HashCommit(t *testing.T) {
	queue := newQueue(&fakeCosi{})

	h := handler{
		Consensus: &Consensus{
			storage: newInMemoryStorage(),
			queue:   queue,
			cosi:    &fakeCosi{},
		},
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	err := h.Consensus.queue.New(forwardLink{to: []byte{0xaa}}, authority)
	require.NoError(t, err)

	buffer, err := h.Invoke(nil, &CommitRequest{To: []byte{0xaa}})
	require.NoError(t, err)
	require.Equal(t, []byte{fake.SignatureByte}, buffer)

	h.cosi = &fakeCosi{factory: fake.NewBadSignatureFactory()}
	_, err = h.Invoke(nil, &CommitRequest{})
	require.EqualError(t, err, "couldn't decode prepare signature: fake error")

	h.cosi = &fakeCosi{}
	queue.locked = false
	_, err = h.Invoke(nil, &CommitRequest{To: []byte("unknown")})
	require.EqualError(t, err, "couldn't update signature: couldn't find proposal '756e6b6e6f776e'")

	h.cosi = &fakeCosi{factory: fake.NewSignatureFactory(fake.NewBadSignature())}
	_, err = h.Invoke(nil, &CommitRequest{To: []byte{0xaa}})
	require.EqualError(t, err, "couldn't marshal the signature: fake error")
}

func TestRPCHandler_Process(t *testing.T) {
	h := rpcHandler{
		validator: fakeValidator{},
		Consensus: &Consensus{
			storage: newInMemoryStorage(),
			queue:   fakeQueue{},
			cosi:    &fakeCosi{},
		},
	}

	resp, err := h.Process(mino.Request{Message: &empty.Empty{}})
	require.EqualError(t, err, "message type not supported: *empty.Empty")
	require.Nil(t, resp)

	req := mino.Request{Message: &PropagateRequest{}}
	resp, err = h.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)

	h.cosi = &fakeCosi{factory: fake.NewBadSignatureFactory()}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't decode commit signature: fake error")

	h.cosi = &fakeCosi{}
	h.Consensus.queue = fakeQueue{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't finalize: oops")

	h.Consensus.queue = fakeQueue{}
	h.Consensus.storage = fakeStorage{}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't write forward link: oops")

	h.Consensus.storage = newInMemoryStorage()
	h.validator = fakeValidator{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't commit: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func checkSignatureValue(t *testing.T, pb *any.Any) {
	var wrapper wrappers.BytesValue
	err := ptypes.UnmarshalAny(pb, &wrapper)
	require.NoError(t, err)
	require.Equal(t, []byte{fake.SignatureByte}, wrapper.GetValue())
}

func makeConsensus(t *testing.T, n int,
	p consensus.Proposal) ([]*Consensus, []consensus.Actor, fake.CollectiveAuthority) {

	manager := minoch.NewManager()

	mm := make([]mino.Mino, n)
	for i := range mm {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)
		mm[i] = m
	}

	ca := fake.NewAuthorityFromMino(bls.NewSigner, mm...)
	cons := make([]*Consensus, n)
	actors := make([]consensus.Actor, n)
	for i, m := range mm {
		cosi := flatcosi.NewFlat(m, ca.GetSigner(i))

		c := NewCoSiPBFT(m, cosi, nil)
		actor, err := c.Listen(fakeValidator{proposal: p})
		require.NoError(t, err)

		cons[i] = c
		actors[i] = actor
	}

	return cons, actors, ca
}

type badCosiActor struct {
	cosi.CollectiveSigning
	delay int
}

func (cs *badCosiActor) Sign(ctx context.Context, pb encoding.Packable,
	ca crypto.CollectiveAuthority) (crypto.Signature, error) {

	if cs.delay > 0 {
		cs.delay--
		return fake.Signature{}, nil
	}

	return fake.NewBadSignature(), nil
}

type fakeStorage struct {
	Storage
}

func (s fakeStorage) Store(*ForwardLinkProto) error {
	return xerrors.New("oops")
}

func (s fakeStorage) ReadChain(id Digest) ([]*ForwardLinkProto, error) {
	return nil, xerrors.New("oops")
}

func (s fakeStorage) ReadLast() (*ForwardLinkProto, error) {
	return nil, xerrors.New("oops")
}

type fakeViewChange struct {
	viewchange.ViewChange
	leader uint32
	denied bool
}

func (vc fakeViewChange) Wait(consensus.Proposal, crypto.CollectiveAuthority) (uint32, bool) {
	return vc.leader, !vc.denied
}

func (vc fakeViewChange) Verify(consensus.Proposal, crypto.CollectiveAuthority) uint32 {
	return vc.leader
}

type fakeGovernance struct {
	viewchange.Governance
	authority    fake.CollectiveAuthority
	changeset    viewchange.ChangeSet
	err          error
	errChangeSet error
}

func (gov fakeGovernance) GetAuthority(index uint64) (viewchange.EvolvableAuthority, error) {
	return gov.authority, gov.err
}

func (gov fakeGovernance) GetChangeSet(consensus.Proposal) (viewchange.ChangeSet, error) {
	return gov.changeset, gov.errChangeSet
}

type fakeQueue struct {
	Queue
	err  error
	call *fake.Call
}

func (q fakeQueue) Clear() {
	q.call.Add("clear")
}

func (q fakeQueue) Finalize(to Digest, commit crypto.Signature) (*ForwardLinkProto, error) {
	return &ForwardLinkProto{}, q.err
}

type badChainFactory struct{}

func (f badChainFactory) FromProto(proto.Message) (consensus.Chain, error) {
	return nil, xerrors.New("oops")
}

type fakeProposal struct {
	consensus.Proposal
	hash     []byte
	previous []byte
	err      error
}

func (p fakeProposal) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, p.err
}

func (p fakeProposal) GetIndex() uint64 {
	return 1
}

func (p fakeProposal) GetHash() []byte {
	return p.hash
}

func (p fakeProposal) GetPreviousHash() []byte {
	return p.previous
}

func (p fakeProposal) GetVerifier() crypto.Verifier {
	return &fakeVerifier{}
}

type fakeValidator struct {
	err      error
	proposal consensus.Proposal
}

func (v fakeValidator) Validate(addr mino.Address,
	msg proto.Message) (consensus.Proposal, error) {

	return v.proposal, v.err
}

func (v fakeValidator) Commit(id []byte) error {
	return v.err
}

type fakeCosi struct {
	cosi.CollectiveSigning
	handler         cosi.Reactor
	err             error
	factory         fake.SignatureFactory
	verifierFactory fake.VerifierFactory
	signer          fake.Signer
}

func (cs *fakeCosi) GetSigner() crypto.Signer {
	return cs.signer
}

func (cs *fakeCosi) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return fake.PublicKeyFactory{}
}

func (cs *fakeCosi) GetSignatureFactory() crypto.SignatureFactory {
	return cs.factory
}

func (cs *fakeCosi) GetVerifierFactory() crypto.VerifierFactory {
	return cs.verifierFactory
}

func (cs *fakeCosi) Listen(h cosi.Reactor) (cosi.Actor, error) {
	cs.handler = h
	return nil, cs.err
}

type fakeCosiActor struct {
	calls []map[string]interface{}
	count uint64
	delay int
	err   error
}

func (a *fakeCosiActor) Sign(ctx context.Context, msg encoding.Packable,
	ca crypto.CollectiveAuthority) (crypto.Signature, error) {

	// Increase the counter each time a test sign a message.
	a.count++
	// Store the call parameters so that they can be verified in the test.
	a.calls = append(a.calls, map[string]interface{}{
		"message": msg,
		"signers": ca,
	})
	if a.err != nil {
		if a.delay == 0 {
			return nil, a.err
		}
		a.delay--
	}
	return fake.Signature{}, nil
}

type fakeRPC struct {
	mino.RPC

	calls []map[string]interface{}
	err   error
}

func (rpc *fakeRPC) Call(ctx context.Context, pb proto.Message,
	memship mino.Players) (<-chan proto.Message, <-chan error) {

	msgs := make(chan proto.Message)
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(msgs)
	}()

	errs := make(chan error, 1)
	if rpc.err != nil {
		errs <- rpc.err
	}
	rpc.calls = append(rpc.calls, map[string]interface{}{
		"message":    pb,
		"membership": memship,
	})
	return msgs, errs
}

type fakeMino struct {
	mino.Mino
	name string
	h    mino.Handler
	err  error
}

func (m *fakeMino) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	m.name = name
	m.h = h
	return nil, m.err
}

type fakeChain struct {
	consensus.Chain
}
