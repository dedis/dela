package skipchain

import (
	"bytes"
	"encoding/binary"
	fmt "fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
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

func TestSkipBlock_GetIndex(t *testing.T) {
	f := func(index uint64) bool {
		block := SkipBlock{Index: index}
		return index == block.GetIndex()
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
		packed, err := block.Pack(encoding.NewProtoEncoder())
		require.NoError(t, err)

		pblock := packed.(*BlockProto)

		require.Equal(t, block.Index, pblock.Index)
		require.Equal(t, block.BackLink.Bytes(), pblock.GetBacklink())
		require.Equal(t, block.GenesisID.Bytes(), pblock.GetGenesisID())

		_, err = block.Pack(fake.BadMarshalAnyEncoder{})
		require.EqualError(t, err, "couldn't marshal the payload: fake error")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_Hash(t *testing.T) {
	block := SkipBlock{
		Payload: &empty.Empty{},
	}

	enc := encoding.NewProtoEncoder()

	_, err := block.computeHash(fake.NewHashFactory(fake.NewBadHashWithDelay(0)), enc)
	require.EqualError(t, err, "couldn't write index: fake error")

	_, err = block.computeHash(fake.NewHashFactory(fake.NewBadHashWithDelay(1)), enc)
	require.EqualError(t, err, "couldn't write genesis hash: fake error")

	_, err = block.computeHash(fake.NewHashFactory(fake.NewBadHashWithDelay(2)), enc)
	require.EqualError(t, err, "couldn't write backlink: fake error")

	_, err = block.computeHash(fake.NewHashFactory(fake.NewBadHashWithDelay(3)), enc)
	require.EqualError(t, err,
		"couldn't write payload: stable serialization failed: fake error")
}

func TestSkipBlock_HashUniqueness(t *testing.T) {
	// This test will detect any field added in the SkipBlock structure but
	// not in the hash function. Then it is either added in the hash, or
	// whitelisted in the test. The field should first be set with a value
	// different from the zero of the type.

	block := SkipBlock{
		Index:     1,
		GenesisID: Digest{1},
		BackLink:  Digest{1},
		Payload:   &wrappers.StringValue{Value: "deadbeef"},
	}

	enc := encoding.NewProtoEncoder()

	prevHash, err := block.computeHash(crypto.NewSha256Factory(), enc)
	require.NoError(t, err)

	value := reflect.ValueOf(&block)

	for i := 0; i < value.Elem().NumField(); i++ {
		field := value.Elem().Field(i)

		if !field.CanSet() {
			// ignore private fields.
			continue
		}

		fieldName := value.Elem().Type().Field(i).Name

		field.Set(reflect.Zero(value.Elem().Field(i).Type()))
		newBlock := value.Interface()

		hash, err := newBlock.(*SkipBlock).computeHash(crypto.NewSha256Factory(), enc)
		require.NoError(t, err)

		errMsg := fmt.Sprintf("field %#v produced same hash", fieldName)
		require.NotEqual(t, prevHash, hash, errMsg)

		prevHash = hash
	}
}

func TestSkipBlock_String(t *testing.T) {
	block := SkipBlock{Index: 5, hash: Digest{1}}
	require.Equal(t, block.String(), "Block[5:0100000000000000]")
}

func TestVerifiableBlock_Pack(t *testing.T) {
	f := func(block SkipBlock) bool {
		vb := VerifiableBlock{
			SkipBlock: block,
			Chain:     fakeChain{},
		}

		packed, err := vb.Pack(encoding.NewProtoEncoder())
		require.NoError(t, err)
		require.IsType(t, (*VerifiableBlockProto)(nil), packed)

		_, err = vb.Pack(fake.BadPackEncoder{})
		require.EqualError(t, err, "couldn't pack block: fake error")

		_, err = vb.Pack(fake.BadPackAnyEncoder{})
		require.EqualError(t, err, "couldn't pack chain: fake error")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromPrevious(t *testing.T) {
	f := func(prev SkipBlock) bool {
		factory := blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		}

		block, err := factory.fromPrevious(prev, &empty.Empty{})
		require.NoError(t, err)
		require.Equal(t, prev.Index+1, block.Index)
		require.Equal(t, prev.GenesisID, block.GenesisID)
		require.Equal(t, prev.GetHash(), block.BackLink.Bytes())

		factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
		_, err = factory.fromPrevious(prev, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't make block: ")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_DecodeBlock(t *testing.T) {
	f := func(block SkipBlock) bool {
		factory := blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		}

		packed, err := block.Pack(encoding.NewProtoEncoder())
		require.NoError(t, err)

		newBlock, err := factory.decodeBlock(packed.(*BlockProto))
		require.NoError(t, err)
		require.Equal(t, block, newBlock)

		_, err = factory.decodeBlock(&empty.Empty{})
		require.EqualError(t, err, "invalid message type '*empty.Empty'")

		factory.encoder = fake.BadUnmarshalDynEncoder{}
		_, err = factory.decodeBlock(&BlockProto{})
		require.EqualError(t, err, "couldn't unmarshal payload: fake error")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockFactory_FromVerifiable(t *testing.T) {
	f := func(block SkipBlock) bool {
		factory := blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
			consensus:   fakeConsensus{hash: block.hash},
		}

		packed, err := block.Pack(encoding.NewProtoEncoder())
		require.NoError(t, err)

		pb := &VerifiableBlockProto{
			Block: packed.(*BlockProto),
			Chain: &any.Any{},
		}

		b, err := factory.FromVerifiable(pb)
		require.NoError(t, err)
		require.NotNil(t, b)

		_, err = factory.FromVerifiable(&empty.Empty{})
		require.EqualError(t, err, "invalid message type '*empty.Empty'")

		factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
		_, err = factory.FromVerifiable(pb)
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't decode the block: ")

		factory.hashFactory = crypto.NewSha256Factory()
		factory.consensus = fakeConsensus{errFactory: xerrors.New("oops")}
		_, err = factory.FromVerifiable(pb)
		require.EqualError(t, err, "couldn't get the chain factory: oops")

		factory.consensus = fakeConsensus{err: xerrors.New("oops")}
		_, err = factory.FromVerifiable(pb)
		require.EqualError(t, err, "couldn't decode the chain: oops")

		factory.consensus = fakeConsensus{hash: Digest{}}
		_, err = factory.FromVerifiable(pb)
		require.EqualError(t, err,
			fmt.Sprintf("mismatch hashes: %#x != %#x", [32]byte{}, block.hash))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func randomUint64(rand *rand.Rand) uint64 {
	buffer := make([]byte, 16)
	rand.Read(buffer)
	return binary.LittleEndian.Uint64(buffer)
}

func (s SkipBlock) Generate(rand *rand.Rand, size int) reflect.Value {
	genesisID := Digest{}
	rand.Read(genesisID[:])

	dataHash := make([]byte, size)
	rand.Read(dataHash)

	backLink := Digest{}
	rand.Read(backLink[:])

	block := SkipBlock{
		Index:     randomUint64(rand),
		GenesisID: genesisID,
		BackLink:  backLink,
		Payload:   &empty.Empty{},
	}

	hash, _ := block.computeHash(crypto.NewSha256Factory(), encoding.NewProtoEncoder())
	block.hash = hash

	return reflect.ValueOf(block)
}

type fakeChain struct {
	consensus.Chain
	hash Digest
	err  error
}

func (c fakeChain) Verify(crypto.Verifier) error {
	return c.err
}

func (c fakeChain) GetLastHash() []byte {
	return c.hash.Bytes()
}

func (c fakeChain) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, c.err
}

type fakeChainFactory struct {
	consensus.ChainFactory
	hash     Digest
	err      error
	errChain error
}

func (f fakeChainFactory) FromProto(proto.Message) (consensus.Chain, error) {
	return fakeChain{hash: f.hash, err: f.errChain}, f.err
}

type fakeConsensus struct {
	consensus.Consensus
	hash       Digest
	err        error
	errChain   error
	errFactory error
	errStore   error
}

func (c fakeConsensus) GetChainFactory() (consensus.ChainFactory, error) {
	return fakeChainFactory{
		hash:     c.hash,
		err:      c.err,
		errChain: c.errChain,
	}, c.errFactory
}

func (c fakeConsensus) GetChain(id []byte) (consensus.Chain, error) {
	return fakeChain{}, c.err
}

func (c fakeConsensus) Listen(consensus.Validator) (consensus.Actor, error) {
	return nil, c.err
}

func (c fakeConsensus) Store(consensus.Chain) error {
	return c.errStore
}
