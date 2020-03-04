package skipchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	fmt "fmt"
	"io"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

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
		protoenc = defaultAnyEncoder{}
	}()

	block := SkipBlock{
		BackLinks:    []BlockID{{}},
		ForwardLinks: []ForwardLink{newTestForwardLink()},
		Payload:      &empty.Empty{},
	}

	e := xerrors.New("pack error")
	protoenc = &testAnyEncoder{err: e}
	_, err := block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewErrAnyEncoding((*BlockHeaderProto)(nil), nil)))

	protoenc = &testAnyEncoder{err: e, delay: 1}
	_, err = block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewErrAnyEncoding((*empty.Empty)(nil), nil)))

	block.ForwardLinks[0] = testForwardLink{err: e}
	_, err = block.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
	require.True(t, xerrors.Is(err, encoding.NewErrEncoding("forward link", nil)))
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
	fl := forwardLink{}

	e := xerrors.New("sig error")
	fl.prepare = testSignature{err: e}
	_, err := fl.Pack()
	require.Error(t, err)
	require.True(t, xerrors.Is(err, e))
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
		forwardlinks[i] = forwardLink{}
	}

	block := SkipBlock{
		Index:         randomUint64(rand),
		Height:        randomUint32(rand),
		BaseHeight:    randomUint32(rand),
		MaximumHeight: randomUint32(rand),
		Roster:        blockchain.SimpleRoster{},
		GenesisID:     genesisID,
		DataHash:      dataHash,
		BackLinks:     []BlockID{{1}},
		ForwardLinks:  forwardlinks,
		Payload:       &empty.Empty{},
	}

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

type testAnyEncoder struct {
	delay int
	err   error
}

func (e *testAnyEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	if e.err != nil {
		if e.delay == 0 {
			return nil, e.err
		}

		e.delay--
	}

	return ptypes.MarshalAny(pb)
}

func (e *testAnyEncoder) UnmarshalAny(any *any.Any, pb proto.Message) error {
	if e.err != nil {
		return e.err
	}

	return ptypes.UnmarshalAny(any, pb)
}

type testForwardLink struct {
	forwardLink
	err error
}

func newTestForwardLink() testForwardLink {
	return testForwardLink{
		forwardLink: forwardLink{
			from: []byte{1},
			to:   []byte{2},
		},
	}
}

func (fl testForwardLink) Pack() (proto.Message, error) {
	if fl.err != nil {
		return nil, fl.err
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

type testVerifier struct {
	err   error
	delay int
	calls []struct {
		msg []byte
		sig crypto.Signature
	}

	crypto.Verifier
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
