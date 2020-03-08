package cosipbft

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/cosi/blscosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
	"golang.org/x/xerrors"
)

func TestConsensus_Basic(t *testing.T) {
	manager := minoch.NewManager()
	m1, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	cosi := blscosi.NewBlsCoSi(m1, blscosi.NewSigner())

	cons := NewCoSiPBFT(m1, cosi)
	err = cons.Listen(fakeValidator{pubkey: cosi.GetPublicKey()})
	require.NoError(t, err)

	prop := fakeProposal{}
	err = cons.Propose(prop, fakeParticipant{
		addr:      m1.GetAddress(),
		publicKey: cosi.GetPublicKey(),
	})
	require.NoError(t, err)

	ch, err := cons.GetChain(prop.GetHash())
	require.NoError(t, err)
	require.Len(t, ch.(forwardLinkChain).links, 1)
}

func TestConsensus_GetChainFactory(t *testing.T) {
	factory := &ChainFactory{}
	cons := &Consensus{
		factory: factory,
	}

	require.Equal(t, factory, cons.GetChainFactory())
}

func TestConsensus_GetChain(t *testing.T) {
	cons := &Consensus{
		storage: newInMemoryStorage(),
	}

	err := cons.storage.Store(&ForwardLinkProto{To: []byte{0xaa}})
	require.NoError(t, err)

	chain, err := cons.GetChain([]byte{0xaa})
	require.NoError(t, err)
	require.Len(t, chain.(forwardLinkChain).links, 1)
}

func TestConsensus_Listen(t *testing.T) {
	fakeCosi := &fakeCosi{}
	fakeMino := &fakeMino{}
	cons := &Consensus{cosi: fakeCosi, mino: fakeMino}

	err := cons.Listen(fakeValidator{})
	require.NoError(t, err)
	require.IsType(t, handler{}, fakeCosi.handler)
	require.IsType(t, rpcHandler{}, fakeMino.h)
	require.Equal(t, rpcName, fakeMino.name)

	err = cons.Listen(nil)
	require.EqualError(t, err, "validator is nil")

	fakeCosi.err = xerrors.New("cosi error")
	err = cons.Listen(fakeValidator{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, fakeCosi.err))

	fakeCosi.err = nil
	fakeMino.err = xerrors.New("rpc error")
	err = cons.Listen(fakeValidator{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, fakeMino.err))
}

func checkSignatureValue(t *testing.T, pb *any.Any, value uint64) {
	var wrapper wrappers.UInt64Value
	err := ptypes.UnmarshalAny(pb, &wrapper)
	require.NoError(t, err)
	require.Equal(t, value, wrapper.GetValue())
}

func TestConsensus_Propose(t *testing.T) {
	cs := &fakeCosi{}
	rpc := &fakeRPC{}
	cons := &Consensus{cosi: cs, rpc: rpc}

	err := cons.Propose(fakeProposal{}, fakeParticipant{})
	require.NoError(t, err)
	require.Len(t, cs.calls, 2)

	prepare := cs.calls[0]["message"].(*Prepare)
	require.NotNil(t, prepare)
	signers := cs.calls[0]["signers"].([]cosi.Cosigner)
	require.Len(t, signers, 1)

	commit := cs.calls[1]["message"].(*Commit)
	require.NotNil(t, commit)
	require.Equal(t, []byte{0xaa}, commit.GetTo())
	checkSignatureValue(t, commit.GetPrepare(), 1)
	signers = cs.calls[1]["signers"].([]cosi.Cosigner)
	require.Len(t, signers, 1)

	require.Len(t, rpc.calls, 1)
	propagate := rpc.calls[0]["message"].(*Propagate)
	require.NotNil(t, propagate)
	require.Equal(t, []byte{0xaa}, propagate.GetTo())
	checkSignatureValue(t, propagate.GetCommit(), 2)
}

func TestConsensus_ProposeFailures(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	cons := &Consensus{}

	e := xerrors.New("pack error")
	err := cons.Propose(fakeProposal{err: e})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewEncodingError("proposal", nil)))

	protoenc = fakeEncoder{}
	err = cons.Propose(fakeProposal{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err,
		encoding.NewAnyEncodingError((*empty.Empty)(nil), nil)))

	protoenc = encoding.NewProtoEncoder()
	err = cons.Propose(fakeProposal{}, fakeNode{})
	require.EqualError(t, err, "node must implement cosi.Cosigner")

	cons.cosi = &fakeCosi{err: xerrors.New("oops")}
	err = cons.Propose(fakeProposal{})
	require.EqualError(t, err, "couldn't sign the proposal: oops")
}

type fakeProposal struct {
	pubkeys []crypto.PublicKey
	err     error
}

func (p fakeProposal) Pack() (proto.Message, error) {
	return &empty.Empty{}, p.err
}

func (p fakeProposal) GetHash() []byte {
	return []byte{0xaa}
}

func (p fakeProposal) GetPublicKeys() []crypto.PublicKey {
	return p.pubkeys
}

type fakeValidator struct {
	pubkey crypto.PublicKey
}

func (v fakeValidator) Validate(msg proto.Message) (consensus.Proposal, consensus.Proposal, error) {
	p := fakeProposal{
		pubkeys: []crypto.PublicKey{v.pubkey},
	}
	return p, p, nil
}

func (v fakeValidator) Commit(id []byte) error {
	return nil
}

type fakeNode struct {
	mino.Node
}

type fakeParticipant struct {
	addr      mino.Address
	publicKey crypto.PublicKey
}

func (p fakeParticipant) GetAddress() mino.Address {
	return p.addr
}

func (p fakeParticipant) GetPublicKey() crypto.PublicKey {
	return p.publicKey
}

type fakeSignature struct {
	crypto.Signature
	value uint64
	err   error
}

func (s fakeSignature) Pack() (proto.Message, error) {
	return &wrappers.UInt64Value{Value: s.value}, s.err
}

func (s fakeSignature) MarshalBinary() ([]byte, error) {
	return []byte{0xde, 0xad, 0xbe, 0xef}, s.err
}

type fakeCosi struct {
	cosi.CollectiveSigning
	handler cosi.Hashable
	calls   []map[string]interface{}
	count   uint64
	err     error
}

func (cs *fakeCosi) Listen(h cosi.Hashable) error {
	cs.handler = h
	return cs.err
}

func (cs *fakeCosi) Sign(pb proto.Message, signers ...cosi.Cosigner) (crypto.Signature, error) {
	// Increase the counter each time a test sign a message.
	cs.count++
	// Store the call parameters so that they can be verified in the test.
	cs.calls = append(cs.calls, map[string]interface{}{
		"message": pb,
		"signers": signers,
	})
	return fakeSignature{value: cs.count}, cs.err
}

type fakeRPC struct {
	mino.RPC

	calls []map[string]interface{}
}

func (rpc *fakeRPC) Call(pb proto.Message, nodes ...mino.Node) (<-chan proto.Message, <-chan error) {
	msgs := make(chan proto.Message, 0)
	close(msgs)
	rpc.calls = append(rpc.calls, map[string]interface{}{
		"message": pb,
		"nodes":   nodes,
	})
	return msgs, nil
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
