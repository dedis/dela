package minogrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestIntegration_BasicLifecycle_Stream(t *testing.T) {

	// Use with MINO_TRAFFIC=log
	// defer func() {
	// 	SaveItems("graph.dot", true, true)
	// 	SaveEvents("events.dot")
	// }()

	mm, rpcs := makeInstances(t, 6, nil)

	authority := fake.NewAuthorityFromMino(fake.NewSigner, mm...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, recv, err := rpcs[0].Stream(ctx, authority)
	require.NoError(t, err)

	iter := authority.AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		err := <-sender.Send(fake.Message{}, to)
		require.NoError(t, err)

		from, msg, err := recv.Recv(context.Background())
		require.NoError(t, err)
		require.Equal(t, to, from)
		require.IsType(t, fake.Message{}, msg)
	}

	// Start the shutdown procedure.
	cancel()

	for _, m := range mm {
		// This makes sure that the relay handlers have been closed by the
		// context.
		require.NoError(t, m.(*Minogrpc).GracefulClose())
	}
}

func TestIntegration_Basic_Call(t *testing.T) {
	call := &fake.Call{}
	mm, rpcs := makeInstances(t, 10, call)

	authority := fake.NewAuthorityFromMino(fake.NewSigner, mm...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpcs[0].Call(ctx, fake.Message{}, authority)
	require.NoError(t, err)

	for {
		select {
		case resp, more := <-resps:
			if !more {
				for _, m := range mm {
					require.NoError(t, m.(*Minogrpc).GracefulClose())
				}

				// Verify the parameter of the Process handler.
				require.Equal(t, 10, call.Len())
				for i := 0; i < 10; i++ {
					req := call.Get(i, 0).(mino.Request)
					require.Equal(t, mm[0].GetAddress(), req.Address)
				}

				return
			}

			msg, err := resp.GetMessageOrError()
			require.NoError(t, err)
			require.Equal(t, fake.Message{}, msg)

		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for closure")
		}
	}
}

func TestOverlayServer_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			tokens:      fakeTokens{},
			certs:       certs.NewInMemoryStore(),
			router:      tree.NewRouter(NewMemship([]mino.Address{}), 3),
			connFactory: fakeConnFactory{},
		},
	}

	cert, err := makeCertificate()
	require.NoError(t, err)

	overlay.certs.Store(fake.NewAddress(0), cert)

	ctx := context.Background()
	req := &JoinRequest{
		Token: "abc",
		Certificate: &Certificate{
			Address: []byte{},
			Value:   cert.Leaf.Raw,
		},
	}

	resp, err := overlay.Join(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	overlay.tokens = fakeTokens{invalid: true}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err, "token 'abc' is invalid")

	overlay.tokens = fakeTokens{}
	overlay.certs.Store(fake.NewBadAddress(), cert)
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err, "couldn't marshal address: fake error")

	overlay.certs = certs.NewInMemoryStore()
	overlay.certs.Store(fake.NewAddress(0), cert)
	overlay.connFactory = fakeConnFactory{err: xerrors.New("oops")}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err,
		"failed to share certificate: couldn't open connection: oops")

	overlay.connFactory = fakeConnFactory{errConn: xerrors.New("oops")}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err,
		"failed to share certificate: couldn't call share: oops")
}

func TestOverlayServer_Share(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			certs:       certs.NewInMemoryStore(),
			router:      tree.NewRouter(NewMemship([]mino.Address{}), 3),
			addrFactory: AddressFactory{},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cert := fake.MakeCertificate(t, 1)

	resp, err := overlay.Share(ctx, &Certificate{Value: cert.Leaf.Raw})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, overlay.certs.Load(address{}))

	_, err = overlay.Share(ctx, &Certificate{})
	require.EqualError(t, err,
		"couldn't parse certificate: asn1: syntax error: sequence truncated")
}

func TestOverlayServer_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			router:      tree.NewRouter(NewMemship([]mino.Address{}), 3),
			context:     json.NewContext(),
			addrFactory: AddressFactory{},
		},
		endpoints: make(map[string]*Endpoint),
	}

	overlay.endpoints["test"] = &Endpoint{Handler: testHandler{}, Factory: fake.MessageFactory{}}
	overlay.endpoints["bad"] = &Endpoint{Handler: mino.UnsupportedHandler{}, Factory: fake.MessageFactory{}}
	overlay.endpoints["bad2"] = &Endpoint{Handler: testHandler{}, Factory: fake.NewBadMessageFactory()}

	md := metadata.New(map[string]string{headerURIKey: "test"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	resp, err := overlay.Call(ctx, &Message{Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []byte(`{}`), resp.GetPayload())

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "unknown"},
	))
	_, err = overlay.Call(badCtx, nil)
	require.EqualError(t, err, "handler 'unknown' is not registered")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad2"},
	))
	_, err = overlay.Call(badCtx, &Message{Payload: []byte(``)})
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad"},
	))
	_, err = overlay.Call(badCtx, &Message{Payload: []byte(``)})
	require.EqualError(t, err, "handler failed to process: rpc is not supported")

	overlay.context = fake.NewBadContext()
	_, err = overlay.Call(ctx, &Message{Payload: []byte(``)})
	require.EqualError(t, err, "couldn't serialize result: fake error")
}

func TestOverlayServer_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			router:      tree.NewRouter(NewMemship([]mino.Address{}), 3),
			context:     json.NewContext(),
			addrFactory: AddressFactory{},
			me:          fake.NewAddress(0),
		},
		closer:    &sync.WaitGroup{},
		endpoints: make(map[string]*Endpoint),
	}

	overlay.endpoints["test"] = &Endpoint{Handler: testHandler{skip: true},
		streams: make(map[string]*StreamSession)}
	overlay.endpoints["bad"] = &Endpoint{Handler: testHandler{skip: true,
		err: xerrors.New("oops")}, streams: make(map[string]*StreamSession)}

	ch := make(chan *Envelope, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	addrBuf, err := newRootAddress().MarshalText()
	require.NoError(t, err)

	inCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test", headerGatewayKey, string(addrBuf), headerStreamIDKey, "test"))

	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.NoError(t, err)

	err = overlay.Stream(fakeServerStream{ctx: ctx})
	require.EqualError(t, err, "handler '' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "unknown"))
	err = overlay.Stream(fakeServerStream{ctx: inCtx})
	require.EqualError(t, err, "handler 'unknown' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test"))
	err = overlay.Stream(fakeServerStream{ctx: inCtx, err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to get gateway, result is nil")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test", headerGatewayKey, string(addrBuf)))
	err = overlay.Stream(fakeServerStream{ctx: inCtx, err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to get streamID, result is empty")

	overlay.context = json.NewContext()
	overlay.router = tree.NewRouter(NewMemship([]mino.Address{}), 3)
	ch = make(chan *Envelope, 1)
	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "bad", headerGatewayKey, string(addrBuf), headerStreamIDKey, "test"))
	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "handler failed to process: oops")
}

func TestOverlay_Join(t *testing.T) {
	cert, err := makeCertificate()
	require.NoError(t, err)

	overlay := overlay{
		me:     fake.NewAddress(0),
		certs:  fakeCerts{},
		router: tree.NewRouter(NewMemship([]mino.Address{}), 3),
		connFactory: fakeConnFactory{
			resp: JoinResponse{Peers: []*Certificate{{Value: cert.Leaf.Raw}}},
		},
		addrFactory: AddressFactory{},
	}

	err = overlay.Join("", "", nil)
	require.NoError(t, err)

	overlay.me = fake.NewBadAddress()
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't marshal own address: fake error")

	overlay.me = fake.NewAddress(0)
	overlay.certs = fakeCerts{err: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't fetch distant certificate: oops")

	overlay.certs = fakeCerts{}
	overlay.connFactory = fakeConnFactory{err: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't open connection: oops")

	overlay.connFactory = fakeConnFactory{resp: JoinResponse{}, errConn: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't call join: oops")

	overlay.connFactory = fakeConnFactory{resp: JoinResponse{Peers: []*Certificate{{}}}}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err,
		"couldn't parse certificate: asn1: syntax error: sequence truncated")
}

func TestConnectionFactory_FromAddress(t *testing.T) {
	dst, err := NewMinogrpc("127.0.0.1", 3334, nil)
	require.NoError(t, err)

	defer dst.GracefulClose()

	factory := DefaultConnectionFactory{
		certs: certs.NewInMemoryStore(),
		me:    fake.NewAddress(0),
	}

	factory.certs.Store(factory.me, &tls.Certificate{})
	factory.certs.Store(dst.GetAddress(), dst.GetCertificate())

	conn, err := factory.FromAddress(dst.GetAddress())
	require.NoError(t, err)
	require.NotNil(t, conn)

	conn.(*grpc.ClientConn).Close()

	_, err = factory.FromAddress(fake.NewAddress(1))
	require.EqualError(t, err, "certificate for 'fake.Address[1]' not found")

	factory.certs.Delete(factory.me)
	_, err = factory.FromAddress(dst.GetAddress())
	require.EqualError(t, err, "couldn't find server 'fake.Address[0]' certificate")

	factory.certs.Store(factory.me, dst.GetCertificate())
	_, err = factory.FromAddress(factory.me)
	require.EqualError(t, err, "invalid address type 'fake.Address'")
}

func TestSenderProcessEnvelope(t *testing.T) {
	me := fake.NewAddress(0)

	inputs := [][]mino.Address{
		{fake.NewAddress(1), fake.NewAddress(2)},
		{fake.NewAddress(1), fake.NewAddress(2), fake.NewAddress(1)},
		{fake.NewAddress(0), fake.NewAddress(2), fake.NewAddress(3), fake.NewAddress(3)},
	}

	expecteds := []map[mino.Address][]mino.Address{
		{
			fake.NewAddress(1): []mino.Address{fake.NewAddress(1)},
			fake.NewAddress(2): []mino.Address{fake.NewAddress(2)},
		},
		{
			fake.NewAddress(1): []mino.Address{fake.NewAddress(1), fake.NewAddress(1)},
			fake.NewAddress(2): []mino.Address{fake.NewAddress(2)},
		},
		{
			fake.NewAddress(2): []mino.Address{fake.NewAddress(2)},
			fake.NewAddress(3): []mino.Address{fake.NewAddress(3), fake.NewAddress(3)},
		},
	}

	for i := range inputs {
		input := inputs[i]
		expected := expecteds[i]

		addrsBuf := make([][]byte, len(input))
		for i, addr := range input {
			addrBuf, err := addr.MarshalText()
			require.NoError(t, err)
			addrsBuf[i] = addrBuf
		}

		envelope := Envelope{
			To: addrsBuf,
		}

		sender := sender{
			me: me,
			receiver: receiver{
				addressFactory: fake.AddressFactory{},
				queue:          newNonBlockingQueue(),
			},
			router: fakeRouter{},
		}

		out, err := sender.processEnvelope(envelope)
		require.NoError(t, err)

		require.Equal(t, len(expected), len(out), "got from dispatch: %v", out)

		for relayAddr, tos := range expected {
			outEnv, found := out[relayAddr]
			require.True(t, found, "%s not found in %v", relayAddr, out)
			require.Equal(t, len(tos), len(outEnv.GetTo()))
			for i, tobuf := range outEnv.GetTo() {
				to := sender.receiver.addressFactory.FromText(tobuf)
				require.True(t, tos[i].Equal(to))
			}
		}
	}

	// Test with a bad router
	tobuf, err := fake.NewAddress(1).MarshalText()
	require.NoError(t, err)

	envelope := Envelope{
		To: [][]byte{tobuf},
	}

	sender := sender{
		me: me,
		receiver: receiver{
			addressFactory: fake.AddressFactory{},
			queue:          newNonBlockingQueue(),
		},
		router: fakeRouter{
			isBad: true,
		},
	}

	_, err = sender.processEnvelope(envelope)
	require.EqualError(t, err, "failed to find route from fake.Address[0] to fake.Address[1]: fake error")
}

func TestSendToRelay(t *testing.T) {
	src, err := NewMinogrpc("127.0.0.1", 3333, nil)
	require.NoError(t, err)

	dst, err := NewMinogrpc("127.0.0.1", 3334, nil)
	require.NoError(t, err)

	defer src.GracefulClose()
	defer dst.GracefulClose()

	factory := DefaultConnectionFactory{
		certs: certs.NewInMemoryStore(),
		me:    src.GetAddress(),
	}

	factory.certs.Store(factory.me, src.GetCertificate())
	factory.certs.Store(dst.GetAddress(), dst.GetCertificate())

	tobuf, err := dst.GetAddress().MarshalText()
	require.NoError(t, err)

	envelope := Envelope{
		To: [][]byte{tobuf},
	}

	sender := sender{
		me: factory.me,
		receiver: receiver{
			addressFactory: fake.AddressFactory{},
			queue:          newNonBlockingQueue(),
		},
		connections: make(map[mino.Address]safeRelay),
		connFactory: factory,
		router:      fakeRouter{},
		lock:        new(sync.Mutex),
	}

	err = sender.sendEnvelope(context.Background(), envelope)
	require.Error(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstances(t *testing.T, n int, call *fake.Call) ([]mino.Mino, []mino.RPC) {
	memship := NewMemship([]mino.Address{})
	mm := make([]mino.Mino, n)
	rpcs := make([]mino.RPC, n)
	for i := range mm {
		m, err := NewMinogrpc("127.0.0.1", 3000+uint16(i), tree.NewRouter(memship, 2))
		require.NoError(t, err)

		rpc, err := m.MakeRPC("test", testHandler{call: call}, fake.MessageFactory{})
		require.NoError(t, err)

		mm[i] = m
		rpcs[i] = rpc

		for _, k := range mm[:i] {
			km := k.(*Minogrpc)

			m.GetCertificateStore().Store(k.GetAddress(), km.GetCertificate())
			km.GetCertificateStore().Store(m.GetAddress(), m.GetCertificate())
		}

		memship.Add(m.GetAddress())
	}

	return mm, rpcs
}

type testHandler struct {
	mino.UnsupportedHandler
	call *fake.Call
	skip bool
	err  error
}

// Stream implements mino.Handler. It implements a simple receiver that will
// return the message received and close.
func (h testHandler) Stream(out mino.Sender, in mino.Receiver) error {
	if h.skip {
		return h.err
	}

	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return err
	}

	err = <-out.Send(msg, from)
	if err != nil {
		return err
	}

	return nil
}

func (h testHandler) Process(req mino.Request) (serde.Message, error) {
	h.call.Add(req)

	return req.Message, nil
}

type fakeServerStream struct {
	grpc.ServerStream
	ch  chan *Envelope
	ctx context.Context
	err error
}

func (s fakeServerStream) Context() context.Context {
	return s.ctx
}

func (s fakeServerStream) Send(m *Envelope) error {
	s.ch <- m
	return nil
}

func (s fakeServerStream) Recv() (*Envelope, error) {
	if s.err != nil {
		return nil, s.err
	}

	env := <-s.ch
	return env, nil
}

type fakeTokens struct {
	tokens.Holder
	invalid bool
}

func (t fakeTokens) Verify(string) bool {
	return !t.invalid
}

type fakeCerts struct {
	certs.Storage
	err error
}

func (s fakeCerts) Store(mino.Address, *tls.Certificate) {

}

func (s fakeCerts) Load(mino.Address) *tls.Certificate {
	return &tls.Certificate{Leaf: &x509.Certificate{Raw: []byte{0x89}}}
}

func (s fakeCerts) Fetch(certs.Dialable, []byte) error {
	return s.err
}

type fakeRouter struct {
	isBad bool
}

func (r fakeRouter) MakePacket(me mino.Address, to mino.Address, msg []byte) router.Packet {
	return fakePacket{
		dest: to,
	}
}

func (r fakeRouter) Forward(packet router.Packet) (mino.Address, error) {
	if r.isBad {
		return nil, xerrors.New("fake error")
	}

	return packet.GetDestination(), nil
}

func (r fakeRouter) OnFailure(to mino.Address) error {
	panic("not implemented") // TODO: Implement
}

type fakePacket struct {
	dest mino.Address
}

func (p fakePacket) GetSource() mino.Address {
	panic("not implemented") // TODO: Implement
}

func (p fakePacket) GetDestination() mino.Address {
	return p.dest
}

func (p fakePacket) GetMessage(ctx serde.Context, f serde.Factory) (serde.Message, error) {
	panic("not implemented") // TODO: Implement
}

// type fakeStream struct {
// 	badSend bool
// 	badRecv bool
// 	outputs []*Envelope
// }

// func (r fakeStream) Context() context.Context {
// 	return context.Background()
// }

// func (r fakeStream) Send(*Envelope) error {
// 	if r.badSend {
// 		return xerrors.Errorf("oops")
// 	}

// 	return nil
// }

// func (r *fakeStream) Recv() (*Envelope, error) {
// 	if r.badRecv {
// 		return nil, xerrors.Errorf("oops")
// 	}

// 	if len(r.outputs) == 0 {
// 		return nil, io.EOF
// 	}

// 	out := r.outputs[len(r.outputs)-1]
// 	r.outputs = r.outputs[:len(r.outputs)-1]

// 	return out, nil
// }
