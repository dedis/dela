package controller

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cmd"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"golang.org/x/xerrors"
)

func TestCertAction_Prepare(t *testing.T) {
	action := certAction{}
	buffer, err := action.Prepare(nil)
	require.NoError(t, err)
	require.Nil(t, buffer)
}

func TestCertAction_Execute(t *testing.T) {
	action := certAction{}

	out := new(bytes.Buffer)
	req := cmd.Request{
		Out:      out,
		Injector: cmd.NewInjector(),
	}

	cert := fake.MakeCertificate(t, 1)

	store := certs.NewInMemoryStore()
	store.Store(fake.NewAddress(0), cert)

	req.Injector.Inject(fakeJoinable{certs: store})

	err := action.Execute(req)
	require.NoError(t, err)
	expected := fmt.Sprintf("Address: fake.Address[0] Certificate: %v\n", cert.Leaf.NotAfter)
	require.Equal(t, expected, out.String())

	req.Injector = cmd.NewInjector()
	err = action.Execute(req)
	require.EqualError(t, err,
		"couldn't resolve: couldn't find dependency for 'minogrpc.Joinable'")
}

func TestTokenAction_Prepare(t *testing.T) {
	action := tokenAction{}

	buffer, err := action.Prepare(fakeContext{duration: time.Millisecond})
	require.NoError(t, err)
	require.Equal(t, "{\"Expiration\":1000000}", string(buffer))
}

func TestTokenAction_Execute(t *testing.T) {
	action := tokenAction{}

	out := new(bytes.Buffer)
	req := cmd.Request{
		Out:      out,
		In:       bytes.NewBufferString("{\"Expiration\":1000000}"),
		Injector: cmd.NewInjector(),
	}

	cert := fake.MakeCertificate(t, 1)

	store := certs.NewInMemoryStore()
	store.Store(fake.NewAddress(0), cert)

	hash, err := store.Hash(cert)
	require.NoError(t, err)

	req.Injector.Inject(fakeJoinable{certs: store})

	err = action.Execute(req)
	require.NoError(t, err)

	expected := fmt.Sprintf("--token abc --cert-hash %s\n",
		base64.StdEncoding.EncodeToString(hash))
	require.Equal(t, expected, out.String())

	req.In = new(bytes.Buffer)
	err = action.Execute(req)
	require.EqualError(t, err, "couldn't decode input: EOF")

	req.In = bytes.NewBufferString("{}")
	req.Injector = cmd.NewInjector()
	err = action.Execute(req)
	require.EqualError(t, err,
		"couldn't resolve: couldn't find dependency for 'minogrpc.Joinable'")
}

func TestJoinAction_Prepare(t *testing.T) {
	action := joinAction{}

	buffer, err := action.Prepare(fakeContext{str: "a"})
	require.NoError(t, err)
	require.Equal(t, "{\"Token\":\"a\",\"Addr\":\"a\",\"CertHash\":\"a\"}", string(buffer))
}

func TestJoinAction_Execute(t *testing.T) {
	action := joinAction{}

	req := cmd.Request{
		In:       bytes.NewBufferString("{\"CertHash\":\"YQ==\"}"),
		Injector: cmd.NewInjector(),
	}

	req.Injector.Inject(fakeJoinable{})

	err := action.Execute(req)
	require.NoError(t, err)

	req.In = bytes.NewBufferString("")
	err = action.Execute(req)
	require.EqualError(t, err, "couldn't decode input: EOF")

	req.In = bytes.NewBufferString("{\"CertHash\":\"a\"}")
	err = action.Execute(req)
	require.EqualError(t, err,
		"couldn't decode digest: illegal base64 data at input byte 0")

	req.In = bytes.NewBufferString("{}")
	req.Injector.Inject(fakeJoinable{err: xerrors.New("oops")})
	err = action.Execute(req)
	require.EqualError(t, err, "couldn't join: oops")

	req.In = bytes.NewBufferString("{}")
	req.Injector = cmd.NewInjector()
	err = action.Execute(req)
	require.EqualError(t, err,
		"couldn't resolve: couldn't find dependency for 'minogrpc.Joinable'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeJoinable struct {
	minogrpc.Joinable
	certs certs.Storage
	err   error
}

func (j fakeJoinable) GetCertificate() *tls.Certificate {
	return j.certs.Load(fake.NewAddress(0))
}

func (j fakeJoinable) GetCertificateStore() certs.Storage {
	return j.certs
}

func (j fakeJoinable) Token(time.Duration) string {
	return "abc"
}

func (j fakeJoinable) Join(string, string, []byte) error {
	return j.err
}

type fakeContext struct {
	cmd.Context
	duration time.Duration
	str      string
	num      int
}

func (ctx fakeContext) Duration(string) time.Duration {
	return ctx.duration
}

func (ctx fakeContext) String(string) string {
	return ctx.str
}

func (ctx fakeContext) Int(string) int {
	return ctx.num
}
