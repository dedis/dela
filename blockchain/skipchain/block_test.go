package skipchain

import (
	"bytes"
	"encoding/binary"
	fmt "fmt"
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
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

func TestDigest_Bytes(t *testing.T) {
	f := func(buffer [32]byte) bool {
		id := Digest(buffer)

		return bytes.Equal(id.Bytes(), buffer[:])
	}

	err := quick.Check(f, &quick.Config{})
	require.NoError(t, err)
}

func TestDigest_String(t *testing.T) {
	f := func(buffer [32]byte) bool {
		id := Digest(buffer)

		return id.String() == fmt.Sprintf("%x", buffer)[:16]
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_GetHash(t *testing.T) {
	f := func(block SkipBlock) bool {
		return bytes.Equal(block.GetHash(), block.hash.Bytes())
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
		require.Equal(t, block.GenesisID.Bytes(), pblock.GetGenesisID())
		require.Equal(t, block.DataHash, pblock.GetDataHash())

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_PackFailures(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	block := SkipBlock{
		BackLinks: []Digest{{}},
		Payload:   &empty.Empty{},
	}

	e := xerrors.New("pack error")

	protoenc = &testProtoEncoder{err: e}
	_, err := block.Pack()
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
		Conodes:       []Conode{randomConode()},
		Height:        1,
		BaseHeight:    1,
		MaximumHeight: 1,
		GenesisID:     Digest{1},
		DataHash:      []byte{0},
		BackLinks:     []Digest{{1}, {2}},
	}

	whitelist := map[string]struct{}{
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

func TestSkipBlock_String(t *testing.T) {
	block := SkipBlock{hash: Digest{1}}
	require.Equal(t, block.String(), fmt.Sprintf("Block[0100000000000000]"))
}

type fakeVerifier struct {
	crypto.Verifier
}

func (v fakeVerifier) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return nil
}

type fakeCosi struct {
	cosi.CollectiveSigning
}

func (cosi fakeCosi) GetVerifier(cosi.CollectiveAuthority) crypto.Verifier {
	return fakeVerifier{}
}

func TestBlockFactory_CreateGenesis(t *testing.T) {
	factory := newBlockFactory(fakeCosi{}, nil, nil)

	genesis, err := factory.createGenesis(Conodes{}, nil)
	require.NoError(t, err)
	require.NotNil(t, genesis)
	require.NotNil(t, factory.genesis)

	hash, err := genesis.computeHash()
	require.NoError(t, err)
	require.Equal(t, hash, genesis.hash)

	genesis, err = factory.createGenesis(Conodes{}, &empty.Empty{})
	require.NoError(t, err)
}

func TestBlockFactory_CreateGenesisFailures(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	e := xerrors.New("encode error")
	protoenc = &testProtoEncoder{err: e}
	factory := newBlockFactory(nil, nil, nil)

	_, err := factory.createGenesis(nil, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
}

func TestBlockFactory_FromPrevious(t *testing.T) {
	factory := newBlockFactory(nil, nil, nil)

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
		require.Equal(t, prev.GetHash(), block.BackLinks[0].Bytes())

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromPreviousFailures(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	factory := newBlockFactory(nil, nil, nil)

	e := xerrors.New("encoding error")
	protoenc = &testProtoEncoder{err: e}

	_, err := factory.fromPrevious(SkipBlock{}, nil)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
}

type fakePublicKeyFactory struct {
	crypto.PublicKeyFactory
}

func (f fakePublicKeyFactory) FromProto(pb proto.Message) (crypto.PublicKey, error) {
	return nil, nil
}

func TestBlockFactory_FromBlock(t *testing.T) {
	factory := newBlockFactory(fakeCosi{}, nil, nil)

	f := func(block SkipBlock) bool {
		packed, err := block.Pack()
		require.NoError(t, err)

		newBlock, err := factory.decodeBlock(fakePublicKeyFactory{}, packed.(*BlockProto))
		require.NoError(t, err)
		require.Equal(t, block, newBlock)

		return reflect.DeepEqual(block, newBlock)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromBlockFailures(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	gen := SkipBlock{}.Generate(rand.New(rand.NewSource(time.Now().Unix())), 5)
	block := gen.Interface().(SkipBlock)

	factory := newBlockFactory(nil, nil, nil)

	src, err := block.Pack()
	require.NoError(t, err)

	e := xerrors.New("encoding error")

	protoenc = &testProtoEncoder{err: e}
	_, err = factory.decodeBlock(nil, src.(*BlockProto))
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*ptypes.DynamicAny)(nil), nil)), err.Error())
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

func randomConode() Conode {
	buffer := make([]byte, 4)
	rand.Read(buffer)
	return Conode{
		addr:      fakeAddress{id: buffer},
		publicKey: fakePublicKey{},
	}
}

func (s SkipBlock) Generate(rand *rand.Rand, size int) reflect.Value {
	genesisID := Digest{}
	rand.Read(genesisID[:])

	dataHash := make([]byte, size)
	rand.Read(dataHash)

	backlinks := make([]Digest, rand.Int31n(int32(size)))
	for i := range backlinks {
		rand.Read(backlinks[i][:])
	}

	block := SkipBlock{
		verifier:      fakeVerifier{},
		Index:         randomUint64(rand),
		Height:        randomUint32(rand),
		BaseHeight:    randomUint32(rand),
		MaximumHeight: randomUint32(rand),
		Conodes:       Conodes{},
		GenesisID:     genesisID,
		DataHash:      dataHash,
		BackLinks:     []Digest{Digest{1}},
		Payload:       &empty.Empty{},
	}

	hash, _ := block.computeHash()
	block.hash = hash

	return reflect.ValueOf(block)
}

type fakeAddress struct {
	id []byte
}

func (a fakeAddress) MarshalText() ([]byte, error) {
	return []byte(a.id), nil
}

func (a fakeAddress) String() string {
	return fmt.Sprintf("%x", a.id)
}

type fakePublicKey struct{}

func (pk fakePublicKey) MarshalBinary() ([]byte, error) {
	return []byte{}, nil
}

func (pk fakePublicKey) Pack() (proto.Message, error) {
	return &empty.Empty{}, nil
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

func (v *testVerifier) Verify(msg []byte, sig crypto.Signature) error {
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
