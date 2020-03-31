package cosipbft

import (
	"context"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
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
	manager := minoch.NewManager()
	m1, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	signer := bls.NewSigner()
	cosi := flatcosi.NewFlat(m1, signer)

	cons := NewCoSiPBFT(m1, cosi)
	actor, err := cons.Listen(fakeValidator{pubkey: signer.GetPublicKey()})
	require.NoError(t, err)

	prop := fakeProposal{}
	err = actor.Propose(prop, fakeCA{
		addrs:   []mino.Address{m1.GetAddress()},
		pubkeys: []crypto.PublicKey{signer.GetPublicKey()},
	})
	require.NoError(t, err)

	ch, err := cons.GetChain(prop.GetHash())
	require.NoError(t, err)
	require.Len(t, ch.(forwardLinkChain).links, 1)
}

func TestConsensus_GetChainFactory(t *testing.T) {
	factory := &chainFactory{}
	cons := &Consensus{chainFactory: factory}

	require.Equal(t, factory, cons.GetChainFactory())
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

func TestConsensus_GetChain(t *testing.T) {
	cons := &Consensus{
		storage:      newInMemoryStorage(),
		chainFactory: newChainFactory(nil),
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

func checkSignatureValue(t *testing.T, pb *any.Any, value uint64) {
	var wrapper wrappers.UInt64Value
	err := ptypes.UnmarshalAny(pb, &wrapper)
	require.NoError(t, err)
	require.Equal(t, value, wrapper.GetValue())
}

func TestActor_Propose(t *testing.T) {
	rpc := &fakeRPC{close: true}
	cosiActor := &fakeCosiActor{}
	actor := &pbftActor{
		encoder:     encoding.NewProtoEncoder(),
		closing:     make(chan struct{}),
		hashFactory: sha256Factory{},
		rpc:         rpc,
		cosiActor:   cosiActor,
	}

	err := actor.Propose(fakeProposal{}, fakeCA{})
	require.NoError(t, err)
	require.Len(t, cosiActor.calls, 2)

	prepare := cosiActor.calls[0]["message"].(*PrepareRequest)
	require.NotNil(t, prepare)
	require.IsType(t, fakeCA{}, cosiActor.calls[0]["signers"])

	commit := cosiActor.calls[1]["message"].(*CommitRequest)
	require.NotNil(t, commit)
	require.Equal(t, []byte{0xaa}, commit.GetTo())
	checkSignatureValue(t, commit.GetPrepare(), 1)
	require.IsType(t, fakeCA{}, cosiActor.calls[1]["signers"])

	require.Len(t, rpc.calls, 1)
	propagate := rpc.calls[0]["message"].(*PropagateRequest)
	require.NotNil(t, propagate)
	require.Equal(t, []byte{0xaa}, propagate.GetTo())
	checkSignatureValue(t, propagate.GetCommit(), 2)

	rpc.close = false
	require.NoError(t, actor.Close())
	err = actor.Propose(fakeProposal{}, fakeCA{})
	require.NoError(t, err)
}

type badCosiActor struct {
	cosi.CollectiveSigning
	delay int
}

func (cs *badCosiActor) Sign(ctx context.Context, pb cosi.Message,
	ca cosi.CollectiveAuthority) (crypto.Signature, error) {

	if cs.delay > 0 {
		cs.delay--
		return fakeSignature{}, nil
	}

	return fakeSignature{err: xerrors.New("oops")}, nil
}

type badCA struct {
	mino.Players
}

func TestConsensus_ProposeFailures(t *testing.T) {
	actor := &pbftActor{
		encoder:     encoding.NewProtoEncoder(),
		hashFactory: sha256Factory{},
	}

	err := actor.Propose(fakeProposal{}, badCA{})
	require.EqualError(t, err, "cosipbft.badCA should implement cosi.CollectiveAuthority")

	actor.cosiActor = &fakeCosiActor{err: xerrors.New("oops")}
	err = actor.Propose(fakeProposal{}, fakeCA{})
	require.EqualError(t, err, "couldn't sign the proposal: oops")

	actor.cosiActor = &badCosiActor{}
	err = actor.Propose(fakeProposal{}, fakeCA{})
	require.EqualError(t, xerrors.Unwrap(err), "couldn't marshal prepare signature: oops")

	actor.cosiActor = &fakeCosiActor{err: xerrors.New("oops"), delay: 1}
	err = actor.Propose(fakeProposal{}, fakeCA{})
	require.EqualError(t, err, "couldn't sign the commit: oops")

	actor.cosiActor = &fakeCosiActor{}
	actor.encoder = badPackAnyEncoder{}
	err = actor.Propose(fakeProposal{}, fakeCA{})
	require.EqualError(t, err, "encoder: oops")

	actor.encoder = encoding.NewProtoEncoder()
	actor.cosiActor = &fakeCosiActor{}
	actor.rpc = &fakeRPC{err: xerrors.New("oops")}
	err = actor.Propose(fakeProposal{}, fakeCA{})
	require.EqualError(t, err, "couldn't propagate the link: oops")
}

func TestActor_Close(t *testing.T) {
	actor := &pbftActor{
		closing: make(chan struct{}),
	}

	require.NoError(t, actor.Close())
	_, ok := <-actor.closing
	require.False(t, ok)
}

func TestHandler_HashPrepare(t *testing.T) {
	cons := &Consensus{
		storage:     newInMemoryStorage(),
		queue:       &queue{},
		hashFactory: crypto.NewSha256Factory(),
		encoder:     encoding.NewProtoEncoder(),
	}
	h := handler{
		validator: fakeValidator{},
		Consensus: cons,
	}

	_, err := h.Hash(nil, &empty.Empty{})
	require.EqualError(t, err, "message type not supported: *empty.Empty")

	empty, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	buffer, err := h.Hash(nil, &PrepareRequest{Proposal: empty})
	require.NoError(t, err)
	require.NotEmpty(t, buffer)

	cons.encoder = badUnmarshalDynEncoder{}
	_, err = h.Hash(nil, &PrepareRequest{})
	require.EqualError(t, err, "encoder: oops")

	cons.encoder = encoding.NewProtoEncoder()
	h.validator = fakeValidator{err: xerrors.New("oops")}
	_, err = h.Hash(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't validate the proposal: oops")

	h.validator = fakeValidator{}
	cons.storage = fakeStorage{}
	_, err = h.Hash(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't read last: oops")

	cons.storage = newInMemoryStorage()
	cons.storage.Store(&ForwardLinkProto{To: []byte{0xaa}})
	_, err = h.Hash(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "mismatch with previous link: aa != bb")

	cons.storage = newInMemoryStorage()
	cons.queue = &queue{locked: true}
	_, err = h.Hash(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't add to queue: queue is locked")

	cons.queue = &queue{}
	cons.hashFactory = badHashFactory{}
	_, err = h.Hash(nil, &PrepareRequest{Proposal: empty})
	require.EqualError(t, err, "couldn't compute hash: couldn't write 'from': oops")
}

func TestHandler_HashCommit(t *testing.T) {
	queue := newQueue()

	h := handler{
		Consensus: &Consensus{
			storage: newInMemoryStorage(),
			queue:   queue,
			cosi:    &fakeCosi{},
		},
	}

	err := h.Consensus.queue.New(fakeProposal{})
	require.NoError(t, err)

	buffer, err := h.Hash(nil, &CommitRequest{To: []byte{0xaa}})
	require.NoError(t, err)
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, buffer)

	h.cosi = &fakeCosi{err: xerrors.New("oops")}
	_, err = h.Hash(nil, &CommitRequest{})
	require.EqualError(t, err, "couldn't decode prepare signature: oops")

	h.cosi = &fakeCosi{}
	queue.locked = false
	_, err = h.Hash(nil, &CommitRequest{To: []byte("unknown")})
	require.EqualError(t, err, "couldn't update signature: couldn't find proposal '756e6b6e6f776e'")

	h.cosi = &fakeCosi{errSig: xerrors.New("oops")}
	_, err = h.Hash(nil, &CommitRequest{To: []byte{0xaa}})
	require.EqualError(t, err, "couldn't marshal the signature: oops")
}

type fakeQueue struct {
	Queue
	err error
}

func (q fakeQueue) Finalize(to Digest, commit crypto.Signature) (*ForwardLinkProto, error) {
	return &ForwardLinkProto{}, q.err
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

	h.cosi = &fakeCosi{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't decode commit signature: oops")

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

type badChainFactory struct{}

func (f badChainFactory) FromProto(proto.Message) (consensus.Chain, error) {
	return nil, xerrors.New("oops")
}

type fakeProposal struct {
	err error
}

func (p fakeProposal) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, p.err
}

func (p fakeProposal) GetHash() []byte {
	return []byte{0xaa}
}

func (p fakeProposal) GetPreviousHash() []byte {
	return []byte{0xbb}
}

func (p fakeProposal) GetVerifier() crypto.Verifier {
	return &fakeVerifier{}
}

type fakeValidator struct {
	pubkey crypto.PublicKey
	err    error
}

func (v fakeValidator) Validate(addr mino.Address,
	msg proto.Message) (consensus.Proposal, error) {

	p := fakeProposal{}
	return p, v.err
}

func (v fakeValidator) Commit(id []byte) error {
	return v.err
}

type fakeAddrIterator struct {
	addrs []mino.Address
	index int
}

func (i *fakeAddrIterator) HasNext() bool {
	return i.index+1 < len(i.addrs)
}

func (i *fakeAddrIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.addrs[i.index]
	}
	return nil
}

type fakePKIterator struct {
	pubkeys []crypto.PublicKey
	index   int
}

func (i *fakePKIterator) HasNext() bool {
	return i.index+1 < len(i.pubkeys)
}

func (i *fakePKIterator) GetNext() crypto.PublicKey {
	if i.HasNext() {
		i.index++
		return i.pubkeys[i.index]
	}
	return nil
}

type fakeCA struct {
	cosi.CollectiveAuthority
	addrs   []mino.Address
	pubkeys []crypto.PublicKey
}

func (ca fakeCA) AddressIterator() mino.AddressIterator {
	return &fakeAddrIterator{
		addrs: ca.addrs,
		index: -1,
	}
}

func (ca fakeCA) PublicKeyIterator() crypto.PublicKeyIterator {
	return &fakePKIterator{
		pubkeys: ca.pubkeys,
		index:   -1,
	}
}

func (ca fakeCA) Len() int {
	return len(ca.addrs)
}

type fakeSignature struct {
	crypto.Signature
	value uint64
	err   error
}

func (s fakeSignature) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &wrappers.UInt64Value{Value: s.value}, nil
}

func (s fakeSignature) MarshalBinary() ([]byte, error) {
	return []byte{0xde, 0xad, 0xbe, 0xef}, s.err
}

type fakeCosi struct {
	cosi.CollectiveSigning
	handler cosi.Hashable
	err     error
	errSig  error
}

func (cs *fakeCosi) GetSignatureFactory() crypto.SignatureFactory {
	return fakeSignatureFactory{err: cs.err, errSig: cs.errSig}
}

func (cs *fakeCosi) Listen(h cosi.Hashable) (cosi.Actor, error) {
	cs.handler = h
	return nil, cs.err
}

type fakeCosiActor struct {
	calls []map[string]interface{}
	count uint64
	delay int
	err   error
}

func (a *fakeCosiActor) Sign(ctx context.Context, msg cosi.Message,
	ca cosi.CollectiveAuthority) (crypto.Signature, error) {

	packed, err := msg.Pack(encoding.NewProtoEncoder())
	if err != nil {
		return nil, err
	}

	// Increase the counter each time a test sign a message.
	a.count++
	// Store the call parameters so that they can be verified in the test.
	a.calls = append(a.calls, map[string]interface{}{
		"message": packed,
		"signers": ca,
	})
	if a.err != nil {
		if a.delay == 0 {
			return nil, a.err
		}
		a.delay--
	}
	return fakeSignature{value: a.count}, nil
}

type fakeRPC struct {
	mino.RPC

	calls []map[string]interface{}
	err   error
	close bool
}

func (rpc *fakeRPC) Call(ctx context.Context, pb proto.Message,
	memship mino.Players) (<-chan proto.Message, <-chan error) {

	msgs := make(chan proto.Message)
	errs := make(chan error, 1)
	if rpc.err != nil {
		errs <- rpc.err
	} else if rpc.close {
		close(msgs)
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
