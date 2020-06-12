package skipchain

import (
	"bytes"
	"encoding/binary"
	fmt "fmt"
	"io"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
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

func TestSkipBlock_VisitJSON(t *testing.T) {
	block := SkipBlock{
		Index:     5,
		GenesisID: Digest{1},
		BackLink:  Digest{2},
		Payload:   fake.Message{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(block)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":5,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":{}}`, string(data))

	_, err = block.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize payload: fake error")
}

func TestSkipBlock_Fingerprint(t *testing.T) {
	block := SkipBlock{
		Index:     1,
		GenesisID: Digest{2},
		BackLink:  Digest{3},
		Payload:   fake.Message{Digest: []byte{1, 2}},
	}

	out := new(bytes.Buffer)
	err := block.Fingerprint(out)
	require.NoError(t, err)
	// Digest length = 8 + 32 + 32 + 2
	require.Equal(t, 74, out.Len())

	err = block.Fingerprint(fake.NewBadHashWithDelay(0))
	require.EqualError(t, err, "couldn't write index: fake error")

	err = block.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write genesis hash: fake error")

	err = block.Fingerprint(fake.NewBadHashWithDelay(2))
	require.EqualError(t, err, "couldn't write backlink: fake error")

	block.Payload = badPayload{}
	err = block.Fingerprint(fake.NewBadHashWithDelay(3))
	require.EqualError(t, err, "couldn't fingerprint payload: oops")
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
		Payload:   fake.Message{Digest: []byte{1}},
	}

	h := crypto.NewSha256Factory().New()
	err := block.Fingerprint(h)
	require.NoError(t, err)

	prevHash := h.Sum(nil)

	value := reflect.ValueOf(&block)

	for i := 0; i < value.Elem().NumField(); i++ {
		field := value.Elem().Field(i)

		fieldName := value.Elem().Type().Field(i).Name

		if !field.CanSet() || fieldName == "UnimplementedMessage" {
			// ignore private fields.
			continue
		}

		fieldValue := reflect.ValueOf(value.Elem().Field(i).Interface())

		field.Set(reflect.Zero(fieldValue.Type()))
		newBlock := value.Interface()

		h := crypto.NewSha256Factory().New()
		err := newBlock.(*SkipBlock).Fingerprint(h)
		require.NoError(t, err)

		hash := h.Sum(nil)

		errMsg := fmt.Sprintf("field %#v produced same hash", fieldName)
		require.NotEqual(t, prevHash, hash, errMsg)

		prevHash = hash
	}
}

func TestSkipBlock_String(t *testing.T) {
	block := SkipBlock{Index: 5, hash: Digest{1}}
	require.Equal(t, block.String(), "Block[5:0100000000000000]")
}

func TestVerifiableBlock_VisitJSON(t *testing.T) {
	vb := VerifiableBlock{
		SkipBlock: SkipBlock{
			Payload: fake.Message{},
		},
		Chain: fakeChain{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(vb)
	require.NoError(t, err)
	expected := `{"Block":{"Index":0,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":{}},"Chain":{}}`
	require.Regexp(t, expected, string(data))

	_, err = vb.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize block: fake error")

	_, err = vb.VisitJSON(fake.NewBadSerializerWithDelay(1))
	require.EqualError(t, err, "couldn't serialize chain: fake error")
}

func TestBlockFactory_VisitJSON(t *testing.T) {
	factory := NewBlockFactory(fake.MessageFactory{})

	ser := json.NewSerializer()

	var block SkipBlock
	err := ser.Deserialize([]byte(`{}`), factory, &block)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize payload: fake error")

	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = ser.Deserialize([]byte(`{}`), factory, &block)
	require.EqualError(t, xerrors.Unwrap(err),
		"couldn't fingerprint block: couldn't write index: fake error")
}

func TestVerifiableFactory_VisitJSON(t *testing.T) {
	factory := NewVerifiableFactory(NewBlockFactory(fake.MessageFactory{}), fakeChainFactory{})

	ser := json.NewSerializer()

	var block VerifiableBlock
	err := ser.Deserialize([]byte(`{"Block":{}}`), factory, &block)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize chain: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializerWithDelay(1)})
	require.EqualError(t, err, "couldn't deserialize block: fake error")
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
		Payload:   fake.Message{},
	}

	h := crypto.NewSha256Factory().New()
	block.Fingerprint(h)
	copy(block.hash[:], h.Sum(nil))

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

func (c fakeChain) GetTo() []byte {
	return c.hash.Bytes()
}

func (c fakeChain) VisitJSON(serde.Serializer) (interface{}, error) {
	return struct{}{}, c.err
}

type fakeChainFactory struct {
	serde.UnimplementedFactory
	hash Digest
}

func (f fakeChainFactory) VisitJSON(serde.FactoryInput) (serde.Message, error) {
	return fakeChain{hash: f.hash}, nil
}

type fakeConsensus struct {
	consensus.Consensus
	hash     Digest
	err      error
	errStore error
}

func (c fakeConsensus) GetChainFactory() serde.Factory {
	return fakeChainFactory{hash: c.hash}
}

func (c fakeConsensus) GetChain(id []byte) (consensus.Chain, error) {
	return fakeChain{}, c.err
}

func (c fakeConsensus) Listen(consensus.Reactor) (consensus.Actor, error) {
	return nil, c.err
}

func (c fakeConsensus) Store(consensus.Chain) error {
	return c.errStore
}

type badPayload struct {
	serde.UnimplementedMessage
}

func (p badPayload) Fingerprint(io.Writer) error {
	return xerrors.New("oops")
}
