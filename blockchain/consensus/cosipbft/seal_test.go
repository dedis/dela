package cosipbft

import (
	"bytes"
	"crypto/sha256"
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
	"golang.org/x/xerrors"
)

func TestForwardLink_GetFrom(t *testing.T) {
	f := func(from []byte) bool {
		seal := forwardLink{from: blockchain.NewBlockID(from)}

		return bytes.Equal(seal.from.Bytes(), seal.GetFrom().Bytes())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestForwardLink_GetTo(t *testing.T) {
	f := func(to []byte) bool {
		seal := forwardLink{to: blockchain.NewBlockID(to)}

		return bytes.Equal(seal.to.Bytes(), seal.GetTo().Bytes())
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
		from:    blockchain.NewBlockID([]byte{1, 2, 3}),
		to:      blockchain.NewBlockID([]byte{4, 5, 6}),
		prepare: testSignature{},
		commit:  testSignature{},
	}

	packed, err := fl.Pack()
	require.NoError(t, err)

	flproto := packed.(*ForwardLinkProto)

	require.Equal(t, fl.GetFrom(), blockchain.NewBlockID(flproto.GetFrom()))
	require.Equal(t, fl.GetTo(), blockchain.NewBlockID(flproto.GetTo()))
}

func TestForwardLink_PackFailures(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

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
		fl := forwardLink{
			from: blockchain.NewBlockID(from),
			to:   blockchain.NewBlockID(to),
		}
		hash, err := fl.computeHash()
		require.NoError(t, err)

		// It is enough for the forward links to have From and To in the hash
		// as the integrity of the block is then verified.
		h := sha256.New()
		h.Write(fl.GetFrom().Bytes())
		h.Write(fl.GetTo().Bytes())
		return bytes.Equal(hash, h.Sum(nil))
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
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

type testProtoEncoder struct {
	encoding.ProtoEncoder
	delay int
	err   error
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
