package skipchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	fmt "fmt"
	"io"
	math "math"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestBlockID_String(t *testing.T) {
	f := func(buffer []byte) bool {
		id := BlockID(buffer)

		return id.String() == fmt.Sprintf("%x", buffer)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_GetID(t *testing.T) {
	f := func(hash []byte) bool {
		block := SkipBlock{
			hash: hash,
		}

		return bytes.Equal(block.GetID(), hash)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_Pack(t *testing.T) {
	f := func(block SkipBlock) bool {
		packed, err := block.Pack()
		if err != nil {
			t.Log(err)
			return false
		}

		pblock := packed.(*blockchain.Block)

		header := &BlockHeaderProto{}
		err = ptypes.UnmarshalAny(pblock.GetHeader(), header)
		require.NoError(t, err)

		require.Equal(t, block.Index, pblock.Index)
		require.Equal(t, block.Height, header.GetHeight())
		require.Equal(t, block.BaseHeight, header.GetBaseHeight())
		require.Equal(t, block.MaximumHeight, header.GetMaximumHeight())
		require.Len(t, header.GetBacklinks(), len(block.BackLinks))
		require.Len(t, header.GetForwardlinks(), len(block.ForwardLinks))
		require.Equal(t, block.GenesisID, BlockID(header.GetGenesisID()))
		require.Equal(t, block.DataHash, header.GetDataHash())

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_PackFailures(t *testing.T) {
	defer func() {
		protoenc = protoEncoder{}
	}()

	block := SkipBlock{
		BackLinks:    []BlockID{{}},
		ForwardLinks: []ForwardLink{newTestForwardLink()},
		Payload:      &empty.Empty{},
	}

	e := xerrors.New("pack error")
	protoenc = &testProtoEncoder{err: e}
	_, err := block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*BlockHeaderProto)(nil), nil)))

	protoenc = &testProtoEncoder{err: e, delay: 1}
	_, err = block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*empty.Empty)(nil), nil)))

	block.ForwardLinks[0] = testForwardLink{err: e}
	_, err = block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewEncodingError("forward link", nil)))
}

func TestSkipBlock_HashUniqueness(t *testing.T) {
	// This test will detect any field added in the SkipBlock structure but
	// not in the hash function. Then it is either added in the hash, or
	// whitelisted in the test. The field should first be set with a value
	// different from the zero of the type.

	block := SkipBlock{
		Index:         1,
		Roster:        testRoster{buffer: []byte{1}},
		Height:        1,
		BaseHeight:    1,
		MaximumHeight: 1,
		GenesisID:     []byte{0},
		DataHash:      []byte{0},
		BackLinks:     []BlockID{{0}},
	}

	whitelist := map[string]struct{}{
		"ForwardLinks": struct{}{},
		"Payload":      struct{}{},
	}

	prevHash, err := block.computeHash()
	require.NoError(t, err)

	value := reflect.ValueOf(&block)

	for i := 0; i < value.Elem().NumField(); i++ {
		field := value.Elem().Field(i)

		if !field.CanSet() {
			// ignore private fields.
			continue
		}

		fieldName := value.Elem().Type().Field(i).Name
		if _, ok := whitelist[fieldName]; ok {
			continue
		}

		field.Set(reflect.Zero(value.Elem().Field(i).Type()))
		newBlock := value.Interface()

		hash, err := newBlock.(*SkipBlock).computeHash()
		require.NoError(t, err)

		errMsg := fmt.Sprintf("field %#v produced same hash", fieldName)
		require.NotEqual(t, prevHash, hash, errMsg)

		prevHash = hash
	}
}

func TestForwardLink_GetFrom(t *testing.T) {
	f := func(from []byte) bool {
		fl := forwardLink{from: from}

		return BlockID(from).String() == fl.GetFrom().String()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestForwardLink_GetTo(t *testing.T) {
	f := func(to []byte) bool {
		fl := forwardLink{to: to}

		return BlockID(to).String() == fl.GetTo().String()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestForwardLink_Verify(t *testing.T) {
	fl := forwardLink{
		hash:    []byte("deadbeef"),
		prepare: testSignature{buffer: []byte{1}},
		commit:  testSignature{buffer: []byte{2}},
	}

	v := &testVerifier{}

	err := fl.Verify(v, nil)
	require.NoError(t, err)
	require.Len(t, v.calls, 2)
	require.Equal(t, fl.hash, v.calls[0].msg)
	require.Equal(t, fl.prepare, v.calls[0].sig)

	sigbuf, err := fl.prepare.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, sigbuf, v.calls[1].msg)
	require.Equal(t, fl.commit, v.calls[1].sig)
}

func TestForwardLink_VerifyFailures(t *testing.T) {
	fl := forwardLink{
		prepare: testSignature{},
		commit:  testSignature{},
	}

	v := &testVerifier{err: xerrors.New("verify error")}

	err := fl.Verify(v, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, v.err))

	v.delay = 1
	err = fl.Verify(v, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, v.err))

	v.err = nil
	e := xerrors.New("marshal error")
	fl.prepare = testSignature{err: e}
	err = fl.Verify(v, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
}

func TestForwardLink_Pack(t *testing.T) {
	fl := forwardLink{
		from:    []byte{1, 2, 3},
		to:      []byte{4, 5, 6},
		prepare: testSignature{},
		commit:  testSignature{},
	}

	packed, err := fl.Pack()
	require.NoError(t, err)

	flproto := packed.(*ForwardLinkProto)

	require.Equal(t, fl.GetFrom(), BlockID(flproto.GetFrom()))
	require.Equal(t, fl.GetTo(), BlockID(flproto.GetTo()))
}

func TestForwardLink_PackFailures(t *testing.T) {
	defer func() { protoenc = newProtoEncoder() }()

	fl := forwardLink{}

	e := xerrors.New("sig error")
	fl.prepare = testSignature{err: e}
	_, err := fl.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))

	fl.prepare = nil
	fl.commit = testSignature{err: e}
	_, err = fl.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))

	fl.prepare = testSignature{}
	fl.commit = testSignature{}
	protoenc = &testProtoEncoder{err: e}
	_, err = fl.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*empty.Empty)(nil), nil)))

	protoenc = &testProtoEncoder{err: e, delay: 1}
	_, err = fl.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*empty.Empty)(nil), nil)))

}

func TestForwardLink_Hash(t *testing.T) {
	f := func(from, to []byte) bool {
		fl := forwardLink{from: from, to: to}
		hash, err := fl.computeHash()
		require.NoError(t, err)

		// It is enough for the forward links to have From and To in the hash
		// as the integrity of the block is then verified.
		h := sha256.New()
		h.Write(from)
		h.Write(to)
		return bytes.Equal(hash, h.Sum(nil))
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestChain_GetProof(t *testing.T) {
	f := func(blocks []SkipBlock) bool {
		numForwardLink := int(math.Max(0, float64(len(blocks)-1)))

		for i := range blocks[:numForwardLink] {
			blocks[i].ForwardLinks = []ForwardLink{
				forwardLink{from: blocks[i].hash, to: blocks[i+1].hash},
			}
		}

		proof := Chain(blocks).GetProof()
		require.Len(t, proof.ForwardLinks, numForwardLink)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestProof_Pack(t *testing.T) {
	proof := Proof{
		ForwardLinks: []ForwardLink{
			forwardLink{from: []byte{0}, to: []byte{1}},
			forwardLink{from: []byte{1}, to: []byte{2}},
		},
	}

	packed, err := proof.Pack()
	require.NoError(t, err)

	msg := packed.(*ProofProto)
	require.Len(t, msg.ForwardLinks, len(proof.ForwardLinks))
	for i := range proof.ForwardLinks {
		require.Equal(t, proof.ForwardLinks[i].GetFrom(), BlockID(msg.ForwardLinks[i].GetFrom()))
		require.Equal(t, proof.ForwardLinks[i].GetTo(), BlockID(msg.ForwardLinks[i].GetTo()))
	}
}

func TestProof_PackFailures(t *testing.T) {
	e := xerrors.New("pack error")
	proof := Proof{
		ForwardLinks: []ForwardLink{
			testForwardLink{err: e},
		},
	}

	_, err := proof.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewEncodingError("forward link", nil)))

	proof.ForwardLinks = []ForwardLink{
		testForwardLink{packed: &empty.Empty{}},
	}

	_, err = proof.Pack()
	require.Error(t, err)
	expectErr := encoding.NewTypeError((*empty.Empty)(nil), (*ForwardLinkProto)(nil))
	require.True(t, xerrors.Is(err, expectErr))
}

func TestProof_Verify(t *testing.T) {
	f := func(blocks []SkipBlock) bool {
		numForwardLink := int(math.Max(0, float64(len(blocks)-1)))

		for i := range blocks[:numForwardLink] {
			blocks[i].ForwardLinks = []ForwardLink{
				forwardLink{
					from:    blocks[i].hash,
					to:      blocks[i+1].hash,
					prepare: testSignature{},
					commit:  testSignature{},
				},
			}
		}

		proof := Chain(blocks).GetProof()
		v := &testVerifier{}

		if len(blocks) > 0 {
			err := proof.Verify(v, blocks[len(blocks)-1])
			require.NoError(t, err)
		}

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestProof_VerifyFailures(t *testing.T) {
	proof := Proof{GenesisBlock: SkipBlock{hash: []byte{123}}}
	err := proof.Verify(nil, SkipBlock{})
	require.Error(t, err)
	require.EqualError(t, err, "mismatch genesis block")

	proof = Proof{ForwardLinks: []ForwardLink{newTestForwardLink()}}
	err = proof.Verify(nil, SkipBlock{})
	require.Error(t, err)
	require.EqualError(t, err, "missing roster in genesis block")

	proof.GenesisBlock.Roster = testRoster{}
	e := xerrors.New("verify error")
	err = proof.Verify(&testVerifier{err: e}, SkipBlock{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e), err.Error())

	proof.GenesisBlock = SkipBlock{hash: []byte{0xaa}, Roster: testRoster{}}
	err = proof.Verify(&testVerifier{}, SkipBlock{})
	require.Error(t, err)
	require.EqualError(t, xerrors.Unwrap(err), "got previous block 01 but expect aa in forward link")

	proof.ForwardLinks = []ForwardLink{
		forwardLink{from: []byte{0xaa}, to: []byte{0xbb}, prepare: testSignature{}, commit: testSignature{}},
		forwardLink{from: []byte{0xcc}, prepare: testSignature{}, commit: testSignature{}},
	}
	err = proof.Verify(&testVerifier{}, SkipBlock{})
	require.Error(t, err)
	require.EqualError(t, xerrors.Unwrap(err), "got previous block cc but expect bb in forward link")

	proof.ForwardLinks = []ForwardLink{
		forwardLink{from: []byte{0xaa}, to: []byte{0xbb}, prepare: testSignature{}, commit: testSignature{}},
	}
	err = proof.Verify(&testVerifier{}, SkipBlock{hash: []byte{0xcc}})
	require.Error(t, err)
	require.EqualError(t, err, "got forward link to bb but expect cc")
}

func TestBlockFactory_CreateGenesis(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	genesis, err := factory.createGenesis(testRoster{}, nil)
	require.NoError(t, err)
	require.NotNil(t, genesis)
	require.NotNil(t, factory.genesis)

	hash, err := genesis.computeHash()
	require.NoError(t, err)
	require.Equal(t, hash, genesis.hash)

	genesis, err = factory.createGenesis(testRoster{}, &empty.Empty{})
	require.NoError(t, err)
}

func TestBlockFactory_CreateGenesisFailures(t *testing.T) {
	defer func() { protoenc = protoEncoder{} }()

	e := xerrors.New("encode error")
	protoenc = &testProtoEncoder{err: e}
	factory := newBlockFactory(&testVerifier{})

	_, err := factory.createGenesis(nil, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
}

func TestBlockFactory_FromPrevious(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	f := func(prev SkipBlock) bool {
		block, err := factory.fromPrevious(prev, &empty.Empty{})
		require.NoError(t, err)
		require.Equal(t, prev.Index+1, block.Index)
		require.Equal(t, prev.Height, block.Height)
		require.Equal(t, prev.BaseHeight, block.BaseHeight)
		require.Equal(t, prev.MaximumHeight, block.MaximumHeight)
		require.Equal(t, prev.GenesisID, block.GenesisID)
		require.NotEqual(t, prev.DataHash, block.DataHash)
		require.Len(t, block.BackLinks, 1)
		require.Equal(t, prev.GetID(), block.BackLinks[0])
		require.Len(t, block.ForwardLinks, 0)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromPreviousFailures(t *testing.T) {
	defer func() { protoenc = newProtoEncoder() }()

	factory := newBlockFactory(&testVerifier{})

	e := xerrors.New("encoding error")
	protoenc = &testProtoEncoder{err: e}

	_, err := factory.fromPrevious(SkipBlock{}, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
}

func TestBlockFactory_FromForwardLink(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	src := &ForwardLinkProto{
		From: []byte{0xaa},
		To:   []byte{0xbb},
	}

	fl, err := factory.fromForwardLink(src)
	require.NoError(t, err)
	require.Equal(t, BlockID(src.GetFrom()), fl.GetFrom())
	require.Equal(t, BlockID(src.GetTo()), fl.GetTo())
	require.NotNil(t, fl.(forwardLink).hash)
	require.NotEqual(t, []byte{}, fl.(forwardLink).hash)

	src.Prepare = &any.Any{}
	src.Commit = &any.Any{}

	fl, err = factory.fromForwardLink(src)
	require.NoError(t, err)
	require.NotNil(t, fl.(forwardLink).prepare)
	require.NotNil(t, fl.(forwardLink).commit)
}

func TestBlockFactory_FromForwardLinkFailures(t *testing.T) {
	e := xerrors.New("sig factory error")
	factory := newBlockFactory(&testVerifier{err: e})

	src := &ForwardLinkProto{Prepare: &any.Any{}}
	_, err := factory.fromForwardLink(src)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewDecodingError("prepare signature", nil)))

	src = &ForwardLinkProto{Commit: &any.Any{}}
	_, err = factory.fromForwardLink(src)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewDecodingError("commit signature", nil)))
}

func TestBlockFactory_FromBlock(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	f := func(block SkipBlock) bool {
		packed, err := block.Pack()
		require.NoError(t, err)

		nb, err := factory.fromBlock(packed.(*blockchain.Block))
		newBlock := nb.(SkipBlock)

		require.Equal(t, block, newBlock)

		return reflect.DeepEqual(block, newBlock)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromBlockFailures(t *testing.T) {
	defer func() { protoenc = newProtoEncoder() }()

	gen := SkipBlock{}.Generate(rand.New(rand.NewSource(time.Now().Unix())), 5)
	block := gen.Interface().(SkipBlock)

	factory := newBlockFactory(&testVerifier{})

	src, err := block.Pack()
	require.NoError(t, err)

	e := xerrors.New("encoding error")
	protoenc = &testProtoEncoder{err: e}

	_, err = factory.fromBlock(src.(*blockchain.Block))
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*BlockHeaderProto)(nil), nil)))

	protoenc = &testProtoEncoder{err: e, delay: 1}
	_, err = factory.fromBlock(src.(*blockchain.Block))
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*ptypes.DynamicAny)(nil), nil)), err.Error())
}

func TestBlockFactory_FromProof(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	f := func(blocks []SkipBlock) bool {
		if len(blocks) == 0 {
			return true
		}

		numForwardLink := int(math.Max(0, float64(len(blocks)-1)))

		for i := range blocks[:numForwardLink] {
			fl := forwardLink{from: blocks[i].hash, to: blocks[i+1].hash}
			hash, err := fl.computeHash()
			require.NoError(t, err)
			fl.hash = hash

			blocks[i].ForwardLinks = []ForwardLink{fl}
		}

		proof := Chain(blocks).GetProof()
		packed, err := proof.Pack()
		require.NoError(t, err)

		packedAny, err := protoenc.MarshalAny(packed)
		require.NoError(t, err)

		factory.genesis = &blocks[0]
		decoded, err := factory.fromProof(packedAny)
		require.NoError(t, err)
		require.Equal(t, proof, decoded)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromVerifiable(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	f := func(blocks []SkipBlock) bool {
		if len(blocks) == 0 {
			return true
		}

		numForwardLink := int(math.Max(0, float64(len(blocks)-1)))

		for i := range blocks[:numForwardLink] {
			fl := forwardLink{from: blocks[i].hash, to: blocks[i+1].hash, prepare: testSignature{}, commit: testSignature{}}
			hash, err := fl.computeHash()
			require.NoError(t, err)
			fl.hash = hash

			blocks[i].ForwardLinks = []ForwardLink{fl}
		}

		proof := Chain(blocks).GetProof()
		packed, err := proof.Pack()
		require.NoError(t, err)
		packedAny, err := protoenc.MarshalAny(packed)
		require.NoError(t, err)

		packed, err = blocks[len(blocks)-1].Pack()
		require.NoError(t, err)

		verifiableBlock := &blockchain.VerifiableBlock{
			Block: packed.(*blockchain.Block),
			Proof: packedAny,
		}

		factory.genesis = &blocks[0]
		decoded, err := factory.FromVerifiable(verifiableBlock, nil)
		require.NoError(t, err)
		require.Equal(t, blocks[len(blocks)-1], decoded)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func randomUint64(rand *rand.Rand) uint64 {
	buffer := make([]byte, 16)
	rand.Read(buffer)
	return binary.LittleEndian.Uint64(buffer)
}

func randomUint32(rand *rand.Rand) uint32 {
	buffer := make([]byte, 8)
	rand.Read(buffer)
	return binary.LittleEndian.Uint32(buffer)
}

func (s SkipBlock) Generate(rand *rand.Rand, size int) reflect.Value {
	genesisID := make([]byte, size)
	rand.Read(genesisID)

	dataHash := make([]byte, size)
	rand.Read(dataHash)

	backlinks := make([]BlockID, rand.Int31n(int32(size)))
	for i := range backlinks {
		backlinks[i] = make([]byte, 32)
		rand.Read(backlinks[i])
	}

	forwardlinks := make([]ForwardLink, rand.Int31n(int32(size)))
	for i := range forwardlinks {
		fl := forwardLink{}

		hash, _ := fl.computeHash()
		fl.hash = hash

		forwardlinks[i] = fl
	}

	roster, _ := blockchain.NewRoster(&testVerifier{})

	block := SkipBlock{
		Index:         randomUint64(rand),
		Height:        randomUint32(rand),
		BaseHeight:    randomUint32(rand),
		MaximumHeight: randomUint32(rand),
		Roster:        roster,
		GenesisID:     genesisID,
		DataHash:      dataHash,
		BackLinks:     []BlockID{{1}},
		ForwardLinks:  forwardlinks,
		Payload:       &empty.Empty{},
	}

	hash, _ := block.computeHash()
	block.hash = hash

	return reflect.ValueOf(block)
}

type testRoster struct {
	buffer []byte
}

func (r testRoster) GetConodes() []*blockchain.Conode {
	return nil
}

func (r testRoster) GetAddresses() []*mino.Address {
	return nil
}

func (r testRoster) GetPublicKeys() []crypto.PublicKey {
	return nil
}

func (r testRoster) WriteTo(w io.Writer) (int64, error) {
	w.Write(r.buffer)
	return int64(len(r.buffer)), nil
}

type testProtoEncoder struct {
	delay int
	err   error
}

func (e *testProtoEncoder) Marshal(pb proto.Message) ([]byte, error) {
	if e.err != nil {
		if e.delay == 0 {
			return nil, e.err
		}
		e.delay--
	}

	return proto.Marshal(pb)
}

func (e *testProtoEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	if e.err != nil {
		if e.delay == 0 {
			return nil, e.err
		}
		e.delay--
	}

	return ptypes.MarshalAny(pb)
}

func (e *testProtoEncoder) UnmarshalAny(any *any.Any, pb proto.Message) error {
	if e.err != nil {
		if e.delay == 0 {
			return e.err
		}
		e.delay--
	}

	return ptypes.UnmarshalAny(any, pb)
}

type testForwardLink struct {
	forwardLink
	err    error
	packed proto.Message
}

func newTestForwardLink() testForwardLink {
	return testForwardLink{
		forwardLink: forwardLink{
			from: []byte{1},
			to:   []byte{2},
		},
	}
}

func (fl testForwardLink) Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error {
	return v.Verify(pubkeys, []byte{}, testSignature{})
}

func (fl testForwardLink) Pack() (proto.Message, error) {
	if fl.err != nil {
		return nil, fl.err
	}

	if fl.packed != nil {
		return fl.packed, nil
	}

	return fl.forwardLink.Pack()
}

type testSignature struct {
	buffer []byte
	err    error
}

func (s testSignature) MarshalBinary() ([]byte, error) {
	return s.buffer, s.err
}

func (s testSignature) Pack() (proto.Message, error) {
	return &empty.Empty{}, s.err
}

type testSignatureFactory struct {
	err error
}

func (f testSignatureFactory) FromProto(pb proto.Message) (crypto.Signature, error) {
	return testSignature{}, f.err
}

type testVerifier struct {
	err   error
	delay int
	calls []struct {
		msg []byte
		sig crypto.Signature
	}

	crypto.Verifier
}

func (v *testVerifier) GetSignatureFactory() crypto.SignatureFactory {
	return testSignatureFactory{err: v.err}
}

func (v *testVerifier) Verify(pubkeys []crypto.PublicKey, msg []byte, sig crypto.Signature) error {
	v.calls = append(v.calls, struct {
		msg []byte
		sig crypto.Signature
	}{msg, sig})

	if v.err != nil {
		if v.delay == 0 {
			return v.err
		}
		v.delay--
	}

	return nil
}
