package cosipbft

import (
	"bytes"
	"context"
	fmt "fmt"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

func TestConsensus_Basic(t *testing.T) {
	reactor := &fakeReactor{digest: []byte{0xbb}}
	cons, actors := makeConsensus(t, 3, reactor)

	// 1. Send a fake proposal with the initial authority.
	err := actors[0].Propose(fake.Message{})
	require.NoError(t, err)

	// 2. Send another fake proposal but with a changeset to add the missing
	// player.
	reactor.digest = []byte{0xcc}
	err = actors[0].Propose(fake.Message{})
	require.NoError(t, err)

	// 3. Send another fake proposal but with a changeset to remove the player.
	reactor.digest = []byte{0xdd}
	err = actors[0].Propose(fake.Message{})
	require.NoError(t, err)

	// 4. Send a final fake proposal with the initial authority.
	reactor.digest = []byte{0xee}
	err = actors[0].Propose(fake.Message{})
	require.NoError(t, err)

	chain, err := cons[0].GetChain([]byte{0xee})
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 4)

	ser := json.NewSerializer()
	data, err := ser.Serialize(chain)
	require.NoError(t, err)

	factory := newChainFactory(cons[0].cosi, cons[0].mino, cons[0].viewchange)

	// Make sure the chain can be verified with the roster changes.
	var chain2 consensus.Chain
	err = ser.Deserialize(data, factory, &chain2)
	require.NoError(t, err)
	require.Equal(t, chain.GetTo(), chain2.GetTo())
}

func TestConsensus_GetChainFactory(t *testing.T) {
	cons := &Consensus{
		cosi:       &fakeCosi{},
		mino:       fake.Mino{},
		viewchange: fakeViewChange{},
	}

	require.NotNil(t, cons.GetChainFactory())
}

func TestConsensus_GetChain(t *testing.T) {
	cons := &Consensus{
		storage: newInMemoryStorage(),
	}

	err := cons.storage.Store(forwardLink{to: []byte{0xaa}})
	require.NoError(t, err)

	chain, err := cons.GetChain([]byte{0xaa})
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 1)

	cons.storage = badStorage{errRead: xerrors.New("oops")}
	_, err = cons.GetChain([]byte{})
	require.EqualError(t, err, "couldn't read the chain: oops")
}

func TestConsensus_Listen(t *testing.T) {
	fakeCosi := &fakeCosi{}
	fakeMino := &fakeMino{}
	cons := &Consensus{
		cosi:       fakeCosi,
		mino:       fakeMino,
		viewchange: fakeViewChange{},
	}

	actor, err := cons.Listen(fakeReactor{})
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.IsType(t, reactor{}, fakeCosi.handler)
	require.IsType(t, rpcHandler{}, fakeMino.h)
	require.Equal(t, rpcName, fakeMino.name)

	_, err = cons.Listen(nil)
	require.EqualError(t, err, "validator is nil")

	fakeCosi.err = xerrors.New("cosi error")
	_, err = cons.Listen(fakeReactor{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, fakeCosi.err))

	fakeCosi.err = nil
	fakeMino.err = xerrors.New("rpc error")
	_, err = cons.Listen(fakeReactor{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, fakeMino.err))
}

func TestActor_Propose(t *testing.T) {
	rpc := &fakeRPC{}
	cosiActor := &fakeCosiActor{}
	actor := &pbftActor{
		Consensus: &Consensus{
			hashFactory: crypto.NewSha256Factory(),
			viewchange:  fakeViewChange{},
			cosi:        &fakeCosi{},
			mino:        fake.Mino{},
			storage:     newInMemoryStorage(),
		},
		closing:   make(chan struct{}),
		rpc:       rpc,
		cosiActor: cosiActor,
		reactor:   fakeReactor{digest: []byte{0xaa}},
	}

	actor.viewchange = fakeViewChange{denied: true}
	err := actor.Propose(fake.Message{})
	require.NoError(t, err)

	actor.viewchange = fakeViewChange{denied: false}
	err = actor.Propose(fake.Message{})
	require.NoError(t, err)
	require.Len(t, cosiActor.calls, 2)

	prepare := cosiActor.calls[0]["message"].(Prepare)
	require.NotNil(t, prepare)

	commit := cosiActor.calls[1]["message"].(Commit)
	require.NotNil(t, commit)
	require.Equal(t, []byte{0xaa}, commit.to)

	require.Len(t, rpc.calls, 1)
	factory := propagateFactory{sigFactory: fake.SignatureFactory{}}
	propagate := tmp.FromProto(rpc.calls[0]["message"].(proto.Message), factory).(Propagate)
	require.NotNil(t, propagate)
	require.Equal(t, []byte{0xaa}, propagate.to)

	require.NoError(t, actor.Close())
	err = actor.Propose(fake.Message{})
	require.NoError(t, err)
}

func TestActor_Failures_Propose(t *testing.T) {
	actor := &pbftActor{
		Consensus: &Consensus{
			hashFactory: crypto.NewSha256Factory(),
			viewchange:  fakeViewChange{},
			cosi:        &fakeCosi{},
			mino:        fake.Mino{},
			storage:     newInMemoryStorage(),
		},
		reactor: fakeReactor{digest: []byte{0xa, 0xb}},
	}

	actor.viewchange = fakeViewChange{err: xerrors.New("oops")}
	err := actor.Propose(fake.Message{})
	require.EqualError(t, err, "couldn't read authority for id 0x0a0b: oops")

	actor.viewchange = fakeViewChange{}
	actor.reactor = fakeReactor{err: xerrors.New("oops")}
	err = actor.Propose(fake.Message{})
	require.EqualError(t, err, "couldn't validate proposal: oops")

	actor.reactor = fakeReactor{digest: []byte{0xa, 0xb}}
	actor.storage = badStorage{errRead: xerrors.New("oops")}
	err = actor.Propose(fake.Message{})
	require.EqualError(t, err,
		"couldn't create prepare request: couldn't read chain: oops")

	actor.storage = newInMemoryStorage()
	actor.cosi = &fakeCosi{signer: fake.NewBadSigner()}
	err = actor.Propose(fake.Message{})
	require.EqualError(t, err,
		"couldn't create prepare request: couldn't sign the request: fake error")

	actor.cosi = &fakeCosi{}
	actor.cosiActor = &fakeCosiActor{err: xerrors.New("oops")}
	err = actor.Propose(fake.Message{})
	require.EqualError(t, err, "couldn't sign the proposal: oops")

	actor.cosiActor = &fakeCosiActor{err: xerrors.New("oops"), delay: 1}
	err = actor.Propose(fake.Message{})
	require.EqualError(t, err, "couldn't sign the commit: oops")

	actor.cosiActor = &fakeCosiActor{}
	buffer := new(bytes.Buffer)
	actor.rpc = &fakeRPC{err: xerrors.New("oops")}
	actor.logger = zerolog.New(buffer)
	err = actor.Propose(fake.Message{})
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

func TestHandler_Prepare_Invoke(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)
	cons := &Consensus{
		storage:     newInMemoryStorage(),
		queue:       &queue{cosi: &fakeCosi{}},
		hashFactory: crypto.NewSha256Factory(),
		viewchange:  fakeViewChange{authority: authority},
		cosi:        &fakeCosi{},
	}
	h := reactor{
		reactor:   fakeReactor{},
		Consensus: cons,
	}

	_, err := h.Invoke(nil, fake.Message{})
	require.EqualError(t, err, "message type not supported 'fake.Message'")

	req := Prepare{
		message: fake.Message{},
		chain:   forwardLinkChain{},
	}

	buffer, err := h.Invoke(fake.NewAddress(0), req)
	require.NoError(t, err)
	require.NotEmpty(t, buffer)

	h.storage = badStorage{errStore: xerrors.New("oops")}
	_, err = h.Invoke(nil, req)
	require.EqualError(t, err, "couldn't store previous chain: oops")

	h.storage = newInMemoryStorage()
	h.reactor = fakeReactor{err: xerrors.New("oops")}
	_, err = h.Invoke(nil, req)
	require.EqualError(t, err, "couldn't validate the proposal: oops")

	h.reactor = fakeReactor{errGenesis: xerrors.New("oops")}
	_, err = h.Invoke(nil, req)
	require.EqualError(t, err, "couldn't get genesis id: oops")

	h.reactor = fakeReactor{}
	h.viewchange = fakeViewChange{err: xerrors.New("oops")}
	_, err = h.Invoke(nil, req)
	require.EqualError(t, err, "couldn't verify: oops")

	cons.viewchange = fakeViewChange{authority: fake.NewAuthority(3, func() crypto.AggregateSigner {
		return fake.NewSignerWithPublicKey(fake.NewBadPublicKey())
	})}
	_, err = h.Invoke(fake.NewAddress(0), req)
	require.EqualError(t, err, "couldn't verify signature: fake error")

	cons.viewchange = fakeViewChange{authority: authority}
	_, err = h.Invoke(fake.NewAddress(999), req)
	require.EqualError(t, err, "couldn't find public key for <fake.Address[999]>")

	cons.queue = &queue{locked: true}
	_, err = h.Invoke(fake.NewAddress(0), req)
	require.EqualError(t, err, "couldn't add to queue: queue is locked")

	cons.queue = &queue{cosi: &fakeCosi{}}
	cons.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = h.Invoke(fake.NewAddress(0), req)
	require.EqualError(t, err,
		"couldn't compute hash: couldn't write 'from': fake error")
}

func TestHandler_HashCommit(t *testing.T) {
	queue := newQueue(&fakeCosi{})

	h := reactor{
		Consensus: &Consensus{
			storage: newInMemoryStorage(),
			queue:   queue,
			cosi:    &fakeCosi{},
		},
	}

	authority := fake.NewAuthority(3, fake.NewSigner)
	commit := Commit{
		to:      []byte{0xaa},
		prepare: fake.Signature{},
	}

	err := h.Consensus.queue.New(forwardLink{to: []byte{0xaa}}, authority)
	require.NoError(t, err)

	buffer, err := h.Invoke(nil, commit)
	require.NoError(t, err)
	require.Equal(t, []byte{fake.SignatureByte}, buffer)

	commit.to = []byte("unknown")
	_, err = h.Invoke(nil, commit)
	require.EqualError(t, err, "couldn't update signature: couldn't find proposal '756e6b6e6f776e'")

	commit.to = []byte{0xaa}
	commit.prepare = fake.NewBadSignature()
	_, err = h.Invoke(nil, commit)
	require.EqualError(t, err, "couldn't marshal the signature: fake error")
}

func TestRPCHandler_Process(t *testing.T) {
	h := rpcHandler{
		reactor: fakeReactor{},
		Consensus: &Consensus{
			storage:    newInMemoryStorage(),
			queue:      fakeQueue{},
			cosi:       &fakeCosi{},
			viewchange: fakeViewChange{},
		},
		factory: propagateFactory{sigFactory: fake.SignatureFactory{}},
	}

	req := mino.Request{Message: tmp.ProtoOf(fake.Message{})}
	resp, err := h.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)

	h.factory = fake.MessageFactory{}
	_, err = h.Process(mino.Request{Message: tmp.ProtoOf(fake.Message{})})
	require.EqualError(t, err, "message type not supported 'fake.Message'")

	h.factory = propagateFactory{sigFactory: fake.SignatureFactory{}}
	h.queue = fakeQueue{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't finalize: oops")

	h.queue = fakeQueue{}
	h.viewchange = fakeViewChange{err: xerrors.New("oops"), filter: 1}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't get authority: oops")

	h.viewchange = fakeViewChange{err: xerrors.New("oops"), filter: 2}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't get new authority: oops")

	h.viewchange = fakeViewChange{}
	h.storage = badStorage{errStore: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't write forward link: oops")

	h.storage = newInMemoryStorage()
	h.reactor = fakeReactor{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't commit: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeConsensus(t *testing.T, n int, r consensus.Reactor) ([]*Consensus, []consensus.Actor) {

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

		c := NewCoSiPBFT(m, cosi, fakeViewChange{
			authority: ca,
			filter:    2,
			csFactory: roster.NewChangeSetFactory(m.GetAddressFactory(), cosi.GetPublicKeyFactory()),
		})

		actor, err := c.Listen(r)
		require.NoError(t, err)

		cons[i] = c
		actors[i] = actor
	}

	return cons, actors
}

type badStorage struct {
	Storage
	errStore error
	errRead  error
}

func (s badStorage) Len() uint64 {
	return 0
}

func (s badStorage) Store(forwardLink) error {
	return s.errStore
}

func (s badStorage) StoreChain(consensus.Chain) error {
	return s.errStore
}

func (s badStorage) ReadChain(id Digest) (consensus.Chain, error) {
	return nil, s.errRead
}

type fakeViewChange struct {
	authority fake.CollectiveAuthority
	csFactory serde.Factory
	filter    int
	denied    bool
	err       error
}

func (vc fakeViewChange) GetChangeSetFactory() serde.Factory {
	return vc.csFactory
}

func (vc fakeViewChange) GetAuthority(index uint64) (viewchange.Authority, error) {
	if int(index) == vc.filter && vc.err != nil {
		return nil, vc.err
	}

	if index != 1 && vc.filter > 0 {
		filtered := vc.authority.Take(mino.RangeFilter(0, vc.filter)).(crypto.CollectiveAuthority)
		// Only first two elements for any proposal other than the second.
		return roster.New(filtered), vc.err
	}

	return roster.New(vc.authority), nil
}

func (vc fakeViewChange) Wait() bool {
	return !vc.denied
}

func (vc fakeViewChange) Verify(addr mino.Address, index uint64) (viewchange.Authority, error) {
	if index != 1 && vc.filter > 0 {
		filtered := vc.authority.Take(mino.RangeFilter(0, vc.filter)).(crypto.CollectiveAuthority)
		// Only first two elements for any proposal other than the second.
		return roster.New(filtered), vc.err
	}

	return roster.New(vc.authority), vc.err
}

type fakeQueue struct {
	Queue
	err  error
	call *fake.Call
}

func (q fakeQueue) Clear() {
	q.call.Add("clear")
}

func (q fakeQueue) Finalize(to Digest, commit crypto.Signature) (*forwardLink, error) {
	return &forwardLink{}, q.err
}

type fakeReactor struct {
	fake.MessageFactory

	digest     []byte
	err        error
	errGenesis error
}

func (v fakeReactor) InvokeGenesis() ([]byte, error) {
	return v.digest, v.errGenesis
}

func (v fakeReactor) InvokeValidate(addr mino.Address, msg serde.Message) ([]byte, error) {
	return v.digest, v.err
}

func (v fakeReactor) InvokeCommit(id []byte) error {
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

func (cs *fakeCosi) GetPublicKeyFactory() serde.Factory {
	return fake.PublicKeyFactory{}
}

func (cs *fakeCosi) GetSignatureFactory() serde.Factory {
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

func (a *fakeCosiActor) Sign(ctx context.Context, msg serde.Message,
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
