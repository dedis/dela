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

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const (
	// headerURIKey is the key used in rpc header to pass the handler URI
	headerURIKey        = "apiuri"
	certificateDuration = time.Hour * 24 * 180
)

var (
	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 7 * time.Second
)

type overlayServer struct {
	overlay

	handlers map[string]mino.Handler
	closer   *sync.WaitGroup
}

// Call implements minogrpc.OverlayClient. It processes the request with the
// targeted handler if it exists, otherwise it returns an error.
func (o overlayServer) Call(ctx context.Context, msg *Message) (*Message, error) {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("header not found in provided context")
	}

	apiURI := headers[headerURIKey]
	if len(apiURI) == 0 {
		return nil, xerrors.Errorf("'%s' not found in context header", headerURIKey)
	}

	// If several are provided, only the first one is taken in account.
	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return nil, xerrors.Errorf("handler '%s' is not registered", apiURI[0])
	}

	message, err := o.encoder.UnmarshalDynamicAny(msg.GetPayload())
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	from := o.routingFactory.GetAddressFactory().FromText(msg.GetFrom())

	req := mino.Request{
		Address: from,
		Message: message,
	}

	result, err := handler.Process(req)
	if err != nil {
		return nil, xerrors.Errorf("handler failed to process: %v", err)
	}

	anyResult, err := o.encoder.MarshalAny(result)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal result: %v", err)
	}

	return &Message{Payload: anyResult}, nil
}

// Relay implements minogrpc.OverlayClient. It opens streams according to the
// routing and transmits the message according to their recipient.
func (o overlayServer) Relay(stream Overlay_RelayServer) error {
	o.closer.Add(1)
	defer o.closer.Done()

	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	uri := uriFromContext(stream.Context())

	handler, ok := o.handlers[uri]
	if !ok {
		return xerrors.Errorf("handler '%s' is not registered", uri)
	}

	// Listen on the first message, which should be the routing infos
	msg, err := stream.Recv()
	if err != nil {
		return xerrors.Errorf("failed to receive routing message: %v", err)
	}

	rting, err := o.routingFactory.FromAny(msg.GetMessage().GetPayload())
	if err != nil {
		return xerrors.Errorf("couldn't decode routing: %v", err)
	}

	relayCtx := metadata.NewOutgoingContext(stream.Context(), metadata.Pairs(headerURIKey, uri))

	sender, receiver, err := o.setupRelays(relayCtx, o.me, rting)
	if err != nil {
		return xerrors.Errorf("couldn't setup relays: %v", err)
	}

	o.setupStream(stream, &sender, &receiver, rting.GetParent(o.me))

	err = handler.Stream(sender, receiver)
	if err != nil {
		return xerrors.Errorf("handler failed to process: %v", err)
	}

	// The participant is done but waits for the protocol to end.
	<-stream.Context().Done()

	return nil
}

type relayer interface {
	Context() context.Context
	Send(*Envelope) error
	Recv() (*Envelope, error)
}

type overlay struct {
	encoder        encoding.ProtoMarshaler
	me             mino.Address
	certs          *sync.Map
	routingFactory routing.Factory
	connFactory    ConnectionFactory
	traffic        *traffic
}

func newOverlay(me mino.Address, rf routing.Factory) (overlay, error) {
	cert, err := makeCertificate()
	if err != nil {
		return overlay{}, xerrors.Errorf("failed to make certificate: %v", err)
	}

	certs := &sync.Map{}
	certs.Store(me, cert)

	o := overlay{
		encoder:        encoding.NewProtoEncoder(),
		me:             me,
		certs:          certs,
		routingFactory: rf,
		connFactory: DefaultConnectionFactory{
			certs: certs,
			me:    me,
		},
	}

	switch os.Getenv("MINO_TRAFFIC") {
	case "log":
		o.traffic = newTraffic(me, rf.GetAddressFactory(), ioutil.Discard)
	case "print":
		o.traffic = newTraffic(me, rf.GetAddressFactory(), os.Stdout)
	}

	return o, nil
}

// GetCertificate returns the certificate of the overlay.
func (o overlay) GetCertificate() *tls.Certificate {
	cert, found := o.certs.Load(o.me)
	if !found {
		// This should never and it will panic if it does as this will provoke
		// several issues later on.
		panic("certificate of the overlay must be populated")
	}

	return cert.(*tls.Certificate)
}

// AddCertificate populates the list of public know certificates of the server.
func (o overlay) AddCertificate(addr mino.Address, cert *tls.Certificate) error {
	o.certs.Store(addr, cert)

	return nil
}

func (o overlay) setupRelays(ctx context.Context,
	senderAddr mino.Address, rting routing.Routing) (sender, receiver, error) {

	receiver := receiver{
		addressFactory: o.routingFactory.GetAddressFactory(),
		encoder:        o.encoder,
		errs:           make(chan error, 1),
		queue:          newNonBlockingQueue(),
	}
	sender := sender{
		encoder:        o.encoder,
		me:             senderAddr,
		addressFactory: AddressFactory{},
		rting:          rting,
		clients:        map[mino.Address]chan OutContext{},
		receiver:       &receiver,
		traffic:        o.traffic,
	}

	for _, link := range rting.GetDirectLinks(senderAddr) {
		fabric.Logger.Trace().
			Str("addr", o.me.String()).
			Str("to", link.String()).
			Msg("open relay")

		err := o.setupRelay(ctx, link, &sender, &receiver, rting)
		if err != nil {
			return sender, receiver, xerrors.Errorf("couldn't setup relay to %v: %v", link, err)
		}
	}

	return sender, receiver, nil
}

func (o overlay) setupRelay(ctx context.Context, relay mino.Address,
	sender *sender, receiver *receiver, rting routing.Routing) error {

	conn, err := o.connFactory.FromAddress(relay)
	if err != nil {
		return xerrors.Errorf("couldn't open connection: %v", err)
	}

	cl := NewOverlayClient(conn)

	client, err := cl.Relay(ctx)
	if err != nil {
		return xerrors.Errorf("couldn't open relay: %v", err)
	}

	rtingAny, err := sender.encoder.PackAny(rting)
	if err != nil {
		return xerrors.Errorf("couldn't pack routing: %v", err)
	}

	err = client.Send(&Envelope{Message: &Message{Payload: rtingAny}})
	if err != nil {
		return xerrors.Errorf("couldn't send routing: %v", err)
	}

	o.setupStream(client, sender, receiver, relay)

	return nil
}

func (o overlay) setupStream(stream relayer, sender *sender, receiver *receiver, addr mino.Address) {
	// Relay sender for that connection.
	ch := make(chan OutContext)
	sender.clients[addr] = ch

	go func() {
		for {
			md, more := <-ch
			if !more {
				return
			}

			err := stream.Send(md.Envelope)
			if err == io.EOF {
				close(md.Done)
				return
			}

			if err != nil {
				md.Done <- xerrors.Errorf("couldn't send: %v", err)
				close(md.Done)
				return
			}

			o.traffic.logSend(stream.Context(), sender.me, addr, md.Envelope)
			close(md.Done)
		}
	}()

	// Relay listener for that connection.
	go func() {
		defer close(ch)

		for {
			envelope, err := stream.Recv()
			if err == io.EOF {
				return
			}

			if err != nil {
				receiver.errs <- xerrors.Errorf("couldn't receive on stream: %v", err)
				return
			}

			o.traffic.logRcv(stream.Context(), addr, sender.me, envelope)

			// TODO: do something with error
			sender.sendEnvelope(envelope, nil)
		}
	}()
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
	certs *sync.Map
	me    mino.Address
}

// FromAddress implements minogrpc.ConnectionFactory. It creates a gRPC
// connection from the server to the client.
func (f DefaultConnectionFactory) FromAddress(addr mino.Address) (grpc.ClientConnInterface, error) {
	clientPubCertItf, found := f.certs.Load(addr)
	if !found {
		return nil, xerrors.Errorf("certificate for '%v' not found", addr)
	}

	clientPubCert, ok := clientPubCertItf.(*tls.Certificate)
	if !ok {
		return nil, xerrors.Errorf("invalid certificate type '%T' for '%v'",
			clientPubCertItf, addr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(clientPubCert.Leaf)

	meItf, found := f.certs.Load(f.me)
	if !found {
		return nil, xerrors.Errorf("couldn't find server '%v' certificate", f.me)
	}

	me, ok := meItf.(*tls.Certificate)
	if !ok {
		return nil, xerrors.Errorf("invalid certificate type '%T' for '%v'",
			meItf, f.me)
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
