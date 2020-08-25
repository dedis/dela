package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
)

const (
	// headerURIKey is the key used in rpc header to pass the handler URI
	headerURIKey        = "apiuri"
	headerGatewayKey    = "gateway"
	certificateDuration = time.Hour * 24 * 180
)

var (
	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 7 * time.Second
)

type overlayServer struct {
	overlay

	endpoints *sync.Map
	closer    *sync.WaitGroup
}

func (o overlayServer) Join(ctx context.Context, req *JoinRequest) (*JoinResponse, error) {
	// 1. Check validity of the token.
	if !o.tokens.Verify(req.Token) {
		return nil, xerrors.Errorf("token '%s' is invalid", req.Token)
	}

	dela.Logger.Info().
		Str("from", string(req.GetCertificate().GetAddress())).
		Msg("valid token received")

	// 2. Share certificates to current participants.
	list := make(map[mino.Address][]byte)
	o.certs.Range(func(addr mino.Address, cert *tls.Certificate) bool {
		list[addr] = cert.Leaf.Raw
		return true
	})

	peers := make([]*Certificate, 0, len(list))
	res := make(chan error, 1)

	for to, cert := range list {
		text, err := to.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		msg := &Certificate{Address: text, Value: cert}

		// Prepare the list of known certificates to send back to the new node.
		peers = append(peers, msg)

		// Share the new node certificate with existing peers.
		go func(to mino.Address) {
			conn, err := o.connFactory.FromAddress(to)
			if err != nil {
				res <- xerrors.Errorf("couldn't open connection: %v", err)
				return
			}

			client := NewOverlayClient(conn)

			_, err = client.Share(ctx, req.GetCertificate())
			if err != nil {
				res <- xerrors.Errorf("couldn't call share: %v", err)
				return
			}

			res <- nil
		}(to)
	}

	ack := 0
	for ack < len(peers) {
		err := <-res
		if err != nil {
			return nil, xerrors.Errorf("failed to share certificate: %v", err)
		}

		ack++
	}

	// 3. Return the set of known certificates.
	return &JoinResponse{Peers: peers}, nil
}

func (o overlayServer) Share(ctx context.Context, msg *Certificate) (*CertificateAck, error) {
	// TODO: verify the validity of the certificate by connecting to the distant
	// node but that requires a protection against malicious share.

	from := o.addrFactory.FromText(msg.GetAddress())

	cert, err := x509.ParseCertificate(msg.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't parse certificate: %v", err)
	}

	o.certs.Store(from, &tls.Certificate{Leaf: cert})

	return &CertificateAck{}, nil
}

// Call implements minogrpc.OverlayClient. It processes the request with the
// targeted handler if it exists, otherwise it returns an error.
func (o overlayServer) Call(ctx context.Context, msg *Message) (*Message, error) {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	uri := uriFromContext(ctx)

	// If several are provided, only the first one is taken in account.
	itf, ok := o.endpoints.Load(uri)
	if !ok {
		return nil, xerrors.Errorf("handler '%s' is not registered", uri)
	}
	endpoint, ok := itf.(*Endpoint)
	if !ok {
		return nil, xerrors.Errorf("expected *Endpoint, got '%T'", itf)
	}

	message, err := endpoint.Factory.Deserialize(o.context, msg.GetPayload())
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	from := o.addrFactory.FromText(msg.GetFrom())

	req := mino.Request{
		Address: from,
		Message: message,
	}

	result, err := endpoint.Handler.Process(req)
	if err != nil {
		return nil, xerrors.Errorf("handler failed to process: %v", err)
	}

	if result == nil {
		return &Message{}, nil
	}

	res, err := result.Serialize(o.context)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize result: %v", err)
	}

	return &Message{Payload: res}, nil
}

// Stream implements minogrpc.OverlayClient. It opens streams according to the
// routing and transmits the message according to their recipient.
func (o *overlayServer) Stream(stream Overlay_StreamServer) error {
	o.closer.Add(1)
	defer o.closer.Done()

	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	uri := uriFromContext(stream.Context())

	itf, ok := o.endpoints.Load(uri)
	if !ok {
		return xerrors.Errorf("handler '%s' is not registered", uri)
	}
	endpoint, ok := itf.(*Endpoint)
	if !ok {
		return xerrors.Errorf("expected *Endpoint, got '%T'", itf)
	}

	gateway := gatewayFromContext(stream.Context(), o.addrFactory)
	if gateway == nil {
		return xerrors.Errorf("failed to get gateway, result is nil")
	}

	first := false

	endpoint.Lock()
	if endpoint.sender == nil {
		first = true

		endpoint.receiver = &receiver{
			context:        o.context,
			factory:        endpoint.Factory,
			addressFactory: o.addrFactory,
			errs:           make(chan error, 1),
			queue:          newNonBlockingQueue(),
			ctx:            stream.Context(),
		}

		endpoint.sender = &sender{
			me:             o.me,
			context:        o.context,
			addressFactory: AddressFactory{},
			clients:        map[mino.Address]chan OutContext{},
			receiver:       endpoint.receiver,
			traffic:        o.traffic,

			router:      o.router,
			connFactory: o.connFactory,
			uri:         uri,
			gateway:     gateway,
			relays:      &relays{r: map[string]relayer{}},
		}

		endpoint.sender.relays.Set(newRootAddress().String(), stream)
		endpoint.sender.relays.Set(gateway.String(), stream)
	}
	endpoint.Unlock()

	mebuf, err := o.me.MarshalText()
	if err != nil {
		return xerrors.Errorf("failed to marshal my address: %v", err)
	}

	relayCtx := metadata.NewOutgoingContext(stream.Context(), metadata.Pairs(
		headerURIKey, uri, headerGatewayKey, string(mebuf)))

	go func() {
		for {
			envelope, err := stream.Recv()
			status, ok := status.FromError(err)
			if ok && status.Code() == codes.Canceled {
				return
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				// TODO: handle
				dela.Logger.Fatal().Msgf("failed to receive: %v", err)
			}

			from := gatewayFromContext(stream.Context(), o.addrFactory)
			o.traffic.logRcv(stream.Context(), from, o.me, envelope)

			dispatched, err := dispatchMessage(*envelope, endpoint.sender)
			if err != nil {
				// TODO: handle
				dela.Logger.Fatal().Msgf("failed to dispatch: %v", err)
			}
			sendToRelays(relayCtx, endpoint.sender, dispatched)
		}
	}()

	if first {
		err := endpoint.Handler.Stream(endpoint.sender, endpoint.receiver)
		if err != nil {
			return xerrors.Errorf("handler failed to process: %v", err)
		}

		// The participant is done but waits for the protocol to end.
		<-stream.Context().Done()

		// Since the handler is done, we reset the sender and receiver
		endpoint.Lock()
		endpoint.sender = nil
		endpoint.receiver = nil
		endpoint.Unlock()

		return nil
	}

	// The participant is done but waits for the protocol to end.
	<-stream.Context().Done()

	return nil
}

// dispatchMessage creates multiple envelopes based on the relays needed for
// each recipient. It also notifies the receiver in case the recipient is
// ourself. This function doesn't send any message, it only returns a map that
// will allow us to then send the envelopes and create the necessary relays if
// needed.
func dispatchMessage(envelope Envelope, sender *sender) (map[mino.Address]*Envelope, error) {
	out := map[mino.Address]*Envelope{}

	for _, tobuf := range envelope.GetTo() {
		to := sender.receiver.addressFactory.FromText(tobuf)

		if to.Equal(sender.me) {
			sender.receiver.appendMessage(envelope.GetMessage())
			continue
		}

		packet := sender.router.MakePacket(sender.me, to, envelope.GetMessage().GetPayload())
		relay, err := sender.router.Forward(packet)
		if err != nil {
			return nil, xerrors.Errorf("failed to find route from %s to %s: %v",
				sender.me, to, err)
		}

		env, found := out[relay]
		if !found {
			out[relay] = &Envelope{
				To:      [][]byte{tobuf},
				Message: envelope.GetMessage(),
			}
		} else {
			env.To = append(env.To, tobuf)
		}
	}

	return out, nil
}

// sendToRelays set up the relays if needed and send the envelopes accordingly.
func sendToRelays(relayCtx context.Context, sender *sender, out map[mino.Address]*Envelope) {
	for relay, env := range out {
		relayer, ok := sender.relays.Get(relay.String())
		if !ok {
			conn, err := sender.connFactory.FromAddress(relay)
			if err != nil {
				// TODO: handle
				dela.Logger.Fatal().Msgf("failed to create addr: %v", err)
			}

			cl := NewOverlayClient(conn)

			relayer, err = cl.Stream(relayCtx)
			if err != nil {
				// TODO: handle
				dela.Logger.Fatal().Msgf("failed to call stream: %v", err)
			}

			sender.relays.Set(relay.String(), relayer)

			go listenRelay(relayCtx, relayer, sender, relay)
		}

		if relay.Equal(newRootAddress()) || relay.Equal(sender.gateway) {
			// In fact we set up the map of relay to send back the message to
			// the one that opened the stream to us (our gateway) in case it
			// should be sent to the root or gateway.
			sender.traffic.logSend(relayCtx, sender.me, sender.gateway, env)
		} else {
			sender.traffic.logSend(relayCtx, sender.me, relay, env)
		}

		relayer.Send(env)
	}
}

// listenRelay listens for new messages that would come from a relay that we set
// up and either notify us, or relay the message.
func listenRelay(relayCtx context.Context, relayer relayer,
	sender *sender, distantAddr mino.Address) {

	for {
		envelope, err := relayer.Recv()
		if err == io.EOF {
			return
		}
		status, ok := status.FromError(err)
		if ok && status.Code() == codes.Canceled {
			return
		}
		if err != nil {
			// TODO: handle
			dela.Logger.Fatal().Msgf("failed to receive: %v", err)
		}

		sender.traffic.logRcv(relayer.Context(), distantAddr, sender.me, envelope)

		dispatched, _ := dispatchMessage(*envelope, sender)
		sendToRelays(relayCtx, sender, dispatched)
	}
}

type relayer interface {
	Context() context.Context
	Send(*Envelope) error
	Recv() (*Envelope, error)
}

type overlay struct {
	context     serde.Context
	me          mino.Address
	certs       certs.Storage
	tokens      tokens.Holder
	router      router.Router
	connFactory ConnectionFactory
	traffic     *traffic
	addrFactory mino.AddressFactory
}

func newOverlay(me mino.Address, router router.Router, addrFactory mino.AddressFactory, ctx serde.Context) (overlay, error) {

	cert, err := makeCertificate()
	if err != nil {
		return overlay{}, xerrors.Errorf("failed to make certificate: %v", err)
	}

	certs := certs.NewInMemoryStore()
	certs.Store(me, cert)

	o := overlay{
		context: ctx,
		me:      me,
		tokens:  tokens.NewInMemoryHolder(),
		certs:   certs,
		router:  router,
		connFactory: DefaultConnectionFactory{
			certs: certs,
			me:    me,
		},
		addrFactory: addrFactory,
	}

	switch os.Getenv("MINO_TRAFFIC") {
	case "log":
		o.traffic = newTraffic(me, addrFactory, ioutil.Discard)
	case "print":
		o.traffic = newTraffic(me, addrFactory, os.Stdout)
	}

	return o, nil
}

// GetCertificate returns the certificate of the overlay.
func (o overlay) GetCertificate() *tls.Certificate {
	me := o.certs.Load(o.me)
	if me == nil {
		// This should never happen and it will panic if it does as this will
		// provoke several issues later on.
		panic("certificate of the overlay must be populated")
	}

	return me
}

// AddCertificateStore returns the certificate store.
func (o overlay) GetCertificateStore() certs.Storage {
	return o.certs
}

// Join sends a join request to a distant node with token generated beforehands
// by the later.
func (o overlay) Join(addr, token string, certHash []byte) error {
	target := o.addrFactory.FromText([]byte(addr))

	netAddr, ok := target.(certs.Dialable)
	if !ok {
		return xerrors.Errorf("invalid address type '%T'", target)
	}

	meCert := o.GetCertificate()

	meAddr, err := o.me.MarshalText()
	if err != nil {
		return xerrors.Errorf("couldn't marshal own address: %v", err)
	}

	// Fetch the certificate of the node we want to join. The hash is used to
	// ensure that we get the right certificate.
	err = o.certs.Fetch(netAddr, certHash)
	if err != nil {
		return xerrors.Errorf("couldn't fetch distant certificate: %v", err)
	}

	conn, err := o.connFactory.FromAddress(target)
	if err != nil {
		return xerrors.Errorf("couldn't open connection: %v", err)
	}

	client := NewOverlayClient(conn)

	req := &JoinRequest{
		Token: token,
		Certificate: &Certificate{
			Address: meAddr,
			Value:   meCert.Leaf.Raw,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := client.Join(ctx, req)
	if err != nil {
		return xerrors.Errorf("couldn't call join: %v", err)
	}

	// Update the certificate store with the response from the node we just
	// joined. That will allow the node to communicate with the network.
	for _, raw := range resp.Peers {
		from := o.addrFactory.FromText(raw.GetAddress())

		leaf, err := x509.ParseCertificate(raw.GetValue())
		if err != nil {
			return xerrors.Errorf("couldn't parse certificate: %v", err)
		}

		o.certs.Store(from, &tls.Certificate{Leaf: leaf})
	}

	return nil
}

func makeCertificate() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("couldn't generate the private key: %+v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(certificateDuration),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	buf, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the certificate: %+v", err)
	}

	cert, err := x509.ParseCertificate(buf)
	if err != nil {
		return nil, xerrors.Errorf("couldn't parse the certificate: %+v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{buf},
		PrivateKey:  priv,
		Leaf:        cert,
	}, nil
}

// ConnectionFactory is a factory to open connection to distant addresses.
type ConnectionFactory interface {
	FromAddress(mino.Address) (grpc.ClientConnInterface, error)
}

// DefaultConnectionFactory creates connection for grpc usages.
type DefaultConnectionFactory struct {
	certs certs.Storage
	me    mino.Address
}

// FromAddress implements minogrpc.ConnectionFactory. It creates a gRPC
// connection from the server to the client.
func (f DefaultConnectionFactory) FromAddress(addr mino.Address) (grpc.ClientConnInterface, error) {
	clientPubCert := f.certs.Load(addr)
	if clientPubCert == nil {
		return nil, xerrors.Errorf("certificate for '%v' not found", addr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(clientPubCert.Leaf)

	me := f.certs.Load(f.me)
	if me == nil {
		return nil, xerrors.Errorf("couldn't find server '%v' certificate", f.me)
	}

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*me},
		RootCAs:      pool,
	})

	netAddr, ok := addr.(address)
	if !ok {
		return nil, xerrors.Errorf("invalid address type '%T'", addr)
	}

	// Connecting using TLS and the distant server certificate as the root.
	conn, err := grpc.Dial(netAddr.GetDialAddress(),
		grpc.WithTransportCredentials(ta),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: defaultMinConnectTimeout,
		}),
	)
	if err != nil {
		return nil, xerrors.Errorf("failed to dial: %v", err)
	}

	return conn, nil
}

func uriFromContext(ctx context.Context) string {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	apiURI := headers[headerURIKey]
	if len(apiURI) == 0 {
		return ""
	}

	return apiURI[0]
}

func gatewayFromContext(ctx context.Context, addrFactory mino.AddressFactory) mino.Address {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	gateway := headers[headerGatewayKey]
	if len(gateway) == 0 {
		return nil
	}

	return addrFactory.FromText([]byte(gateway[0]))
}

type relays struct {
	sync.Mutex
	r map[string]relayer
}

func (r *relays) Get(k string) (relayer, bool) {
	r.Lock()
	defer r.Unlock()
	el, ok := r.r[k]
	return el, ok
}

func (r *relays) Set(k string, v relayer) {
	r.Lock()
	defer r.Unlock()
	r.r[k] = v
}
