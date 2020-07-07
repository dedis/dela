package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func init() {
	RegisterBlockFormat(fake.GoodFormat, fake.Format{Msg: SkipBlock{}})
	RegisterBlockFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterVerifiableBlockFormats(fake.GoodFormat, fake.Format{Msg: VerifiableBlock{}})
	RegisterVerifiableBlockFormats(fake.BadFormat, fake.NewBadFormat())
}

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

func TestSkipBlock_Getters(t *testing.T) {
	f := func(index uint64, genesis, back [32]byte) bool {
		block, err := NewSkipBlock(fake.Message{},
			WithIndex(index), WithGenesisID(genesis[:]), WithBackLink(back[:]))

		require.NoError(t, err)
		require.Equal(t, genesis[:], block.GetGenesisID())
		require.Equal(t, back[:], block.GetBackLink())
		require.Equal(t, fake.Message{}, block.GetPayload())

		return index == block.GetIndex()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSkipBlock_WithHashFactory(t *testing.T) {
	block, err := NewSkipBlock(fake.Message{}, WithHashFactory(crypto.NewSha256Factory()))
	require.NoError(t, err)
	require.Len(t, block.GetHash(), 32)

	_, err = NewSkipBlock(fake.Message{}, WithHashFactory(fake.NewHashFactory(fake.NewBadHash())))
	require.EqualError(t, err, "couldn't fingerprint: couldn't write index: fake error")
}

func TestSkipBlock_Serialize(t *testing.T) {
	block := SkipBlock{}

	data, err := block.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = block.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode block: fake error")
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

func TestVerifiableBlock_Serialize(t *testing.T) {
	block := VerifiableBlock{}

	data, err := block.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = block.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode block: fake error")
}

func TestBlockFactory_Deserialize(t *testing.T) {
	factory := NewBlockFactory(fake.MessageFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, SkipBlock{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode block: fake error")
}

func TestVerifiableBlockFactory_Deserialize(t *testing.T) {
	factory := NewVerifiableFactory(fakeChainFactory{}, fake.MessageFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, VerifiableBlock{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode block: fake error")
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

type badPayload struct {
	blockchain.Payload
}

func (p badPayload) Fingerprint(io.Writer) error {
	return xerrors.New("oops")
}

type fakeChainFactory struct {
	consensus.ChainFactory
}
