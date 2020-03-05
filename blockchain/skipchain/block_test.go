package skipchain

import (
	"bytes"
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
	"go.dedis.ch/fabric/blockchain/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestSkipBlock_GetID(t *testing.T) {
	f := func(block SkipBlock) bool {
		return bytes.Equal(block.GetID().Bytes(), block.hash)
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

		pblock := packed.(*BlockProto)

		require.Equal(t, block.Index, pblock.Index)
		require.Equal(t, block.Height, pblock.GetHeight())
		require.Equal(t, block.BaseHeight, pblock.GetBaseHeight())
		require.Equal(t, block.MaximumHeight, pblock.GetMaximumHeight())
		require.Len(t, pblock.GetBacklinks(), len(block.BackLinks))
		require.Len(t, pblock.GetSeals(), len(block.Seals))
		require.Equal(t, block.GenesisID.Bytes(), pblock.GetGenesisID())
		require.Equal(t, block.DataHash, pblock.GetDataHash())

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
		BackLinks: []blockchain.BlockID{{}},
		Seals:     []consensus.Seal{newTestSeal()},
		Payload:   &empty.Empty{},
	}

	e := xerrors.New("pack error")

	block.Seals[0] = testSeal{err: e}
	_, err := block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewEncodingError("forward link", nil)))

	protoenc = &testProtoEncoder{err: e}
	_, err = block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*empty.Empty)(nil), nil)))
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
		GenesisID:     blockchain.NewBlockID([]byte{1}),
		DataHash:      []byte{0},
		BackLinks:     []blockchain.BlockID{blockchain.NewBlockID(nil)},
	}

	whitelist := map[string]struct{}{
		"Seals":   struct{}{},
		"Payload": struct{}{},
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

func TestChain_GetProof(t *testing.T) {
	f := func(blocks []SkipBlock) bool {
		numForwardLink := int(math.Max(0, float64(len(blocks)-1)))

		for i := range blocks[:numForwardLink] {
			blocks[i].Seals = []consensus.Seal{
				testSeal{from: blocks[i].hash, to: blocks[i+1].hash},
			}
		}

		chain := SkipBlocks(blocks).GetProof()
		require.Len(t, chain.seals, numForwardLink)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestProof_Pack(t *testing.T) {
	chain := Chain{
		last: SkipBlock{},
		seals: []consensus.Seal{
			testSeal{from: []byte{0}, to: []byte{1}},
			testSeal{from: []byte{1}, to: []byte{2}},
		},
	}

	packed, err := chain.Pack()
	require.NoError(t, err)

	msg := packed.(*ChainProto)
	require.Len(t, msg.GetSeals(), len(chain.seals))
}

func TestProof_PackFailures(t *testing.T) {
	e := xerrors.New("pack error")
	chain := Chain{
		seals: []consensus.Seal{
			testSeal{err: e},
		},
	}

	_, err := chain.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewEncodingError("forward link", nil)))

	chain.seals = []consensus.Seal{
		testSeal{packed: &empty.Empty{}},
	}

	_, err = chain.Pack()
	require.Error(t, err)
	expectErr := encoding.NewTypeError((*empty.Empty)(nil), (*ChainProto)(nil))
	require.True(t, xerrors.Is(err, expectErr))
}

func TestProof_Verify(t *testing.T) {
	f := func(blocks []SkipBlock) bool {
		numForwardLink := int(math.Max(0, float64(len(blocks)-1)))

		for i := range blocks[:numForwardLink] {
			blocks[i].Seals = []consensus.Seal{
				testSeal{
					from: blocks[i].hash,
					to:   blocks[i+1].hash,
				},
			}
		}

		chain := SkipBlocks(blocks).GetProof()
		v := &testVerifier{}

		if len(blocks) > 0 {
			err := chain.Verify(v)
			require.NoError(t, err)
		}

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestProof_VerifyFailures(t *testing.T) {
	proof := Chain{genesis: SkipBlock{hash: []byte{123}}}
	err := proof.Verify(nil)
	require.Error(t, err)
	require.EqualError(t, err, "mismatch genesis block")

	proof = Chain{seals: []consensus.Seal{newTestSeal()}}
	err = proof.Verify(nil)
	require.Error(t, err)
	require.EqualError(t, err, "missing roster in genesis block")

	proof.genesis.Roster = testRoster{}
	e := xerrors.New("verify error")
	err = proof.Verify(&testVerifier{err: e})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e), err.Error())

	proof.genesis = SkipBlock{hash: []byte{0xaa}, Roster: testRoster{}}
	err = proof.Verify(&testVerifier{})
	require.Error(t, err)
	require.EqualError(t, xerrors.Unwrap(err), "got previous block '01' but expected 'aa' in forward link")

	proof.seals = []consensus.Seal{
		testSeal{from: []byte{0xaa}, to: []byte{0xbb}},
		testSeal{from: []byte{0xcc}},
	}
	err = proof.Verify(&testVerifier{})
	require.Error(t, err)
	require.EqualError(t, xerrors.Unwrap(err), "got previous block 'cc' but expected 'bb' in forward link")

	proof.seals = []consensus.Seal{
		testSeal{from: []byte{0xaa}, to: []byte{0xbb}},
	}
	err = proof.Verify(&testVerifier{})
	require.Error(t, err)
	require.EqualError(t, err, "got forward link to 'bb' but expected 'cc'")
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
		require.Len(t, block.Seals, 0)

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

func TestBlockFactory_FromBlock(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})

	f := func(block SkipBlock) bool {
		packed, err := block.Pack()
		require.NoError(t, err)

		newBlock, err := factory.fromBlock(packed.(*BlockProto))
		require.NoError(t, err)
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

	_, err = factory.fromBlock(src.(*BlockProto))
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*BlockProto)(nil), nil)))

	protoenc = &testProtoEncoder{err: e, delay: 1}
	_, err = factory.fromBlock(src.(*BlockProto))
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*ptypes.DynamicAny)(nil), nil)), err.Error())
}

func TestBlockFactory_FromVerifiable(t *testing.T) {
	factory := newBlockFactory(&testVerifier{})
	factory.sealFactory = testSealFactory{}

	f := func(blocks []SkipBlock) bool {
		if len(blocks) == 0 {
			return true
		}

		chain, err := SkipBlocks(blocks).GetProof().Pack()
		require.NoError(t, err)

		factory.genesis = &blocks[0]
		decoded, err := factory.FromVerifiable(chain, nil)
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
	genesisID := blockchain.BlockID{}
	rand.Read(genesisID[:])

	dataHash := make([]byte, size)
	rand.Read(dataHash)

	backlinks := make([]blockchain.BlockID, rand.Int31n(int32(size)))
	for i := range backlinks {
		rand.Read(backlinks[i][:])
	}

	seals := make([]consensus.Seal, rand.Int31n(int32(size))+1)
	for i := range seals {
		fl := testSeal{}

		seals[i] = fl
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
		BackLinks:     []blockchain.BlockID{blockchain.NewBlockID([]byte{1})},
		Seals:         seals,
		Payload:       &empty.Empty{},
	}

	hash, _ := block.computeHash()
	block.hash = hash

	return reflect.ValueOf(block)
}

type testRoster struct {
	buffer []byte
}

func (r testRoster) GetConodes() ([]*blockchain.Conode, error) {
	return nil, nil
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

type testSeal struct {
	from   []byte
	to     []byte
	err    error
	packed proto.Message
}

func newTestSeal() consensus.Seal {
	return testSeal{}
}

func (s testSeal) GetFrom() blockchain.BlockID {
	return blockchain.NewBlockID(s.from)
}

func (s testSeal) GetTo() blockchain.BlockID {
	return blockchain.NewBlockID(s.to)
}

func (s testSeal) Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error {
	return v.Verify(pubkeys, []byte{}, testSignature{})
}

func (s testSeal) Pack() (proto.Message, error) {
	if s.err != nil {
		return nil, s.err
	}

	if s.packed != nil {
		return s.packed, nil
	}

	return &empty.Empty{}, nil
}

type testSealFactory struct{}

func (f testSealFactory) FromProto(pb proto.Message) (consensus.Seal, error) {
	return testSeal{}, nil
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
