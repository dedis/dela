package cosipbft

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

func TestForwardLink_GetFrom(t *testing.T) {
	f := func(from []byte) bool {
		seal := testSeal{from: from}

		return bytes.Equal(from, seal.GetFrom().Bytes())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestForwardLink_GetTo(t *testing.T) {
	f := func(to []byte) bool {
		seal := testSeal{to: to}

		return bytes.Equal(to, seal.GetTo().Bytes())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestForwardLink_Verify(t *testing.T) {
	fl := testSeal{
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
