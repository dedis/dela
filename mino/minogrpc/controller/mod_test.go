package controller

import (
	"crypto/elliptic"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc"
)

func TestMiniController_Build(t *testing.T) {
	ctrl := NewController()

	call := &fake.Call{}
	ctrl.SetCommands(fakeBuilder{call: call})

	require.Equal(t, 22, call.Len())
}

func TestMiniController_OnStart(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	str := map[string]string{"routing": "flat"}
	paths := map[string]string{"config": dir}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.NoError(t, err)

	str = map[string]string{"routing": "tree"}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.NoError(t, err)

	var m *minogrpc.Minogrpc
	err = injector.Resolve(&m)
	require.NoError(t, err)
	require.NoError(t, m.GracefulStop())
}

func TestMiniController_InvalidAddr_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"listen": ":xxx"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "failed to parse listen URL: parse \":xxx\": missing protocol scheme")
}

func TestMiniController_OverlayFailed_OnStart(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer listener.Close()

	// The address is correct but it will yield an error because it is already
	// used.

	str := map[string]string{"listen": "tcp://" + listener.Addr().String(), "routing": "flat"}
	paths := map[string]string{"config": dir}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.True(t, strings.HasPrefix(err.Error(), "couldn't make overlay: failed to bind"), err.Error())
}

func TestMiniController_MissingDB_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"routing": "flat"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "injector: couldn't find dependency for 'kv.DB'")
}

func TestMiniController_UnknownRouting_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"routing": "fake"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "unknown routing: fake")
}

func TestMiniController_FailGenerateKey_OnStart(t *testing.T) {
	ctrl := NewController().(miniController)
	ctrl.random = badReader{}

	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	str := map[string]string{"routing": "flat"}

	err := ctrl.OnStart(fakeContext{str: str}, inj)
	require.EqualError(t, err,
		fake.Err("cert private key: while loading: generator failed: ecdsa"))
}

func TestMiniController_FailMarshalKey_OnStart(t *testing.T) {
	ctrl := NewController().(miniController)
	ctrl.curve = badCurve{Curve: elliptic.P224()}

	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	str := map[string]string{"routing": "flat"}

	err := ctrl.OnStart(fakeContext{str: str}, inj)
	require.EqualError(t, err,
		"cert private key: while loading: generator failed: while marshaling: x509: unknown elliptic curve")
}

func TestMiniController_FailParseKey_OnStart(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	ctrl := NewController().(miniController)

	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	file, err := os.Create(filepath.Join(dir, certKeyName))
	require.NoError(t, err)

	defer file.Close()

	str := map[string]string{"routing": "flat"}
	paths := map[string]string{"config": dir}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, inj)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cert private key: key parsing failed: ", err.Error())
}

func TestMiniController_LoadCertChain_OnStart(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	str := map[string]string{"routing": "flat"}
	certPath := filepath.Join(dir, "cert.pem")
	paths := map[string]string{"config": dir, "certChain": certPath}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.True(t, strings.HasPrefix(err.Error(), "failed to load certificate:"), err)

	// openssl ecparam -genkey -name secp384r1 -out server.key
	key := []byte(`
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDBeJdT0obeUhS8lkDPcHZWyQPtbT2MzlLWEvwQn0B8l8TY+tKJyHb0N
DWV5URssaGCgBwYFK4EEACKhZANiAASOZ6u/2lP9Q70XZVGAXDsMfBLzqUBT7YbP
1AkJWqQdEkN7ORbrTA/atGLT+mnz3kFSMTZZ9CQppnDnoMcWFSms9znF880z0Dr/
y4MT5nPHp28W9xpgvZuU/0v5xuNerxs=
-----END EC PRIVATE KEY-----
`)
	// openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
	cert := []byte(`
-----BEGIN CERTIFICATE-----
MIICHTCCAaKgAwIBAgIUP2nfWp+QLXcX+wRAqqm433ep/p4wCgYIKoZIzj0EAwIw
RTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoMGElu
dGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMjA3MjUxNTE2NDNaFw0zMjA3MjIx
NTE2NDNaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYD
VQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwdjAQBgcqhkjOPQIBBgUrgQQA
IgNiAASOZ6u/2lP9Q70XZVGAXDsMfBLzqUBT7YbP1AkJWqQdEkN7ORbrTA/atGLT
+mnz3kFSMTZZ9CQppnDnoMcWFSms9znF880z0Dr/y4MT5nPHp28W9xpgvZuU/0v5
xuNerxujUzBRMB0GA1UdDgQWBBT2bYWZ7Uj/6KdAOF0ox/CNvebRwDAfBgNVHSME
GDAWgBT2bYWZ7Uj/6KdAOF0ox/CNvebRwDAPBgNVHRMBAf8EBTADAQH/MAoGCCqG
SM49BAMCA2kAMGYCMQCX6h4/ISxMEmYsodBBZpLsWSdH61V5wbde6jIy7H3YU/iA
7z7ljg9k+ANkSJDLpukCMQC4yxaPKkIjIjpVH8oXqUwwDVEhW7m34q+cWGjTeTKW
SHG2NjtOc3koWHIAr2waQuA=
-----END CERTIFICATE-----
`)

	keyPath := filepath.Join(dir, "key.pem")

	err = os.WriteFile(certPath, cert, 0755)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, key, 0755)
	require.NoError(t, err)

	paths["certKey"] = keyPath

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.NoError(t, err)
}

func TestMiniController_FailedTCPResolve_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"listen": "yyy:xxx"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "failed to resolve tcp address: unknown network yyy")
}

func TestMiniController_FailedPublicParse_OnStart(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	// The address is correct but it will yield an error because it is already
	// used.

	str := map[string]string{"listen": "tcp://1.2.3.4:0", "public": ":xxx", "routing": "flat"}
	paths := map[string]string{"config": dir}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.EqualError(t, err, `failed to parse public: parse ":xxx": missing protocol scheme`)
}

func TestMiniController_OnStop(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController()

	injector := node.NewInjector()
	injector.Inject(db)

	str := map[string]string{"routing": "flat"}
	paths := map[string]string{"config": dir}

	err = ctrl.OnStart(fakeContext{path: paths, str: str}, injector)
	require.NoError(t, err)

	err = ctrl.OnStop(injector)
	require.NoError(t, err)
}

func TestMiniController_MissingMino_OnStop(t *testing.T) {
	ctrl := NewController()

	err := ctrl.OnStop(node.NewInjector())
	require.EqualError(t, err, "injector: couldn't find dependency for 'controller.StoppableMino'")
}

func TestMiniController_FailStopMino_OnStop(t *testing.T) {
	ctrl := NewController()

	inj := node.NewInjector()
	inj.Inject(badMino{})

	err := ctrl.OnStop(inj)
	require.EqualError(t, err, fake.Err("while stopping mino"))
}

func TestGetKey_Pem(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	ctrl := NewController()

	keyPath := filepath.Join(dir, certKeyName)

	// // openssl ecparam -genkey -name secp384r1 -out key.key
	ECKey := []byte(`
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDBeJdT0obeUhS8lkDPcHZWyQPtbT2MzlLWEvwQn0B8l8TY+tKJyHb0N
DWV5URssaGCgBwYFK4EEACKhZANiAASOZ6u/2lP9Q70XZVGAXDsMfBLzqUBT7YbP
1AkJWqQdEkN7ORbrTA/atGLT+mnz3kFSMTZZ9CQppnDnoMcWFSms9znF880z0Dr/
y4MT5nPHp28W9xpgvZuU/0v5xuNerxs=
-----END EC PRIVATE KEY-----
`)

	err = os.WriteFile(keyPath, ECKey, 0755)
	require.NoError(t, err)

	_, err = ctrl.(miniController).getKey(fakeContext{path: map[string]string{"config": dir}})
	require.NoError(t, err)

	// openssl genrsa -out key.pem 1024
	PKCS8Key := []byte(`
-----BEGIN PRIVATE KEY-----
MIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAOigddRqfCewqeht
7PcVbgNyzBjZ8HUHA19FkLUqcuqgYJfSexyGi9N0yeYBOVg+PFRx23nP3t/na88O
9AHf47ztcmYTbU0UZpbu/So2yID/f1v6AegIKK/HZo2OuHJPH2E5r8uXdJLWqRUc
ToYfO/YgTIJ1hEfRL6e7f2NO5UwjAgMBAAECgYA6Za2uuVyZihvdIVtPW63WZ8cc
pflbJ3uNOyVslU9r3v7gnhIRwyTu3G6issP2hwkWGc8C8U/93VaPEC3pGo9Mr7M1
qi8fhwsaVNgbg5J8qaQ6lD7Sdy1ZMz7nkELWRUZ6wlZ+5++5v0pKYoUncm5wVA59
7uZV90hpZKgdAkYXwQJBAPjHR+I71+PlPCoqN+21scxzfSlmeHocLXp9f4QL9ZBV
tuhSPzCiPuJkvyIj6nN36bxjtv41/8CGi3ObYe7xFlkCQQDvYSdR64ijLP7mpUFm
S8HA0avE8jiAi5nBnKP+r59JoyMdv1980G87OMuKA507w4HtC85OuFPtlN8UpG5C
tt7bAkAR84dLWtgcOLlbrYo1m+vFffvlFeDRpuDdOtsNszM4BAdbwjuPDdYNzglA
tGjBhkCWeHeG5mya/tpnMCoj7L+ZAkEAqFkBGCG3JFreoUKTLegVSQ+r54QZrH2B
EqKgytqkAVuTtLYD53mG4HVe358PExq54wWsf7wueiV6hb/mM1D8hQJAIeJoyDha
cuf0orehi7zxdSmj9T217EXJkF4ENr0VcT7uTOOB7gp2KAQVo/xIfvH8F4h3Btae
UpY5smzho+wOGQ==
-----END PRIVATE KEY-----
`)

	err = os.WriteFile(keyPath, PKCS8Key, 0755)
	require.NoError(t, err)

	_, err = ctrl.(miniController).getKey(fakeContext{path: map[string]string{"config": dir}})
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeCommandBuilder struct {
	call *fake.Call
}

func (b fakeCommandBuilder) SetSubCommand(name string) cli.CommandBuilder {
	b.call.Add(name)
	return b
}

func (b fakeCommandBuilder) SetDescription(value string) {
	b.call.Add(value)
}

func (b fakeCommandBuilder) SetFlags(flags ...cli.Flag) {
	b.call.Add(flags)
}

func (b fakeCommandBuilder) SetAction(a cli.Action) {
	b.call.Add(a)
}

type fakeBuilder struct {
	call *fake.Call
}

func (b fakeBuilder) SetCommand(name string) cli.CommandBuilder {
	b.call.Add(name)
	return fakeCommandBuilder(b)
}

func (b fakeBuilder) SetStartFlags(flags ...cli.Flag) {
	b.call.Add(flags)
}

func (b fakeBuilder) MakeAction(tmpl node.ActionTemplate) cli.Action {
	b.call.Add(tmpl)
	return nil
}

type badMino struct {
	StoppableMino
}

func (badMino) GracefulStop() error {
	return fake.GetError()
}

type badReader struct{}

func (badReader) Read([]byte) (int, error) {
	return 0, fake.GetError()
}

type badCurve struct {
	elliptic.Curve
}
