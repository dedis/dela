package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
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
}

// Call is the implementation of the overlay.Call proto definition
func (o overlayServer) Call(ctx context.Context, msg *Message) (*Message, error) {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers[headerURIKey]
	if !ok {
		return nil, xerrors.Errorf("%s not found in context header", headerURIKey)
	}
	if len(apiURI) != 1 {
		return nil, xerrors.Errorf("unexpected number of elements in %s "+
			"header. Expected 1, found %d", headerURIKey, len(apiURI))
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return nil, xerrors.Errorf("didn't find the '%s' handler in the map "+
			"of handlers, did you register it?", apiURI[0])
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
		return nil, xerrors.Errorf("failed to call the Process function from "+
			"the handler using the provided message: %v", err)
	}

	anyResult, err := o.encoder.MarshalAny(result)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal result: %v", err)
	}

	return &Message{Payload: anyResult}, nil
}

func (o overlayServer) Relay(stream Overlay_RelayServer) error {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	ctx := stream.Context()
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers[headerURIKey]
	if !ok {
		return xerrors.Errorf("%s not found in context header", headerURIKey)
	}
	if len(apiURI) != 1 {
		return xerrors.Errorf("unexpected number of elements in apiuri "+
			"header. Expected 1, found %d", len(apiURI))
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return xerrors.Errorf("didn't find the '%s' handler in the map "+
			"of handlers, did you register it?", apiURI[0])
	}

	// Listen on the first message, which should be the routing infos
	msg, err := stream.Recv()
	if err != nil {
		return xerrors.Errorf("failed to receive first routing message: %v", err)
	}

	rting, err := o.routingFactory.FromAny(msg.GetMessage().GetPayload())
	if err != nil {
		return err
	}

	sender, receiver, err := o.setupRelays(headers, o.me, rting)
	if err != nil {
		return xerrors.Errorf("couldn't setup relays: %v", err)
	}

	o.setupStream(stream, &sender, nil)

	err = handler.Stream(sender, receiver)
	if err != nil {
		return err
	}

	// The participant is done but waits for the protocol to end.
	<-stream.Context().Done()

	return nil
}

type relayer interface {
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
		traffic: newTraffic(me.String()),
	}

	return o, nil
}

func (o overlay) GetCertificate() *tls.Certificate {
	cert, found := o.certs.Load(o.me)
	if !found {
		return nil
	}

	return cert.(*tls.Certificate)
}

func (o overlay) setupRelays(hd metadata.MD, senderAddr mino.Address, rting routing.Routing) (sender, receiver, error) {
	receiver := receiver{
		addressFactory: o.routingFactory.GetAddressFactory(),
		encoder:        o.encoder,
		errs:           make(chan error, 1),
		queue: &NonBlockingQueue{
			ch: make(chan *Message, 1),
		},
	}
	sender := sender{
		encoder:        o.encoder,
		me:             senderAddr,
		addressFactory: AddressFactory{},
		rting:          rting,
		clients:        map[mino.Address]chan *Envelope{},
		receiver:       &receiver,
		traffic:        o.traffic,
	}

	for _, link := range rting.GetDirectLinks(senderAddr) {
		err := o.setupRelay(link, &sender, hd, rting)
		if err != nil {
			return sender, receiver, xerrors.Errorf("couldn't setup relay to %v: %v", link, err)
		}
	}

	return sender, receiver, nil
}

func (o overlay) setupRelay(relay mino.Address, sender *sender, hd metadata.MD, rting routing.Routing) error {
	conn, err := o.connFactory.FromAddress(relay)
	if err != nil {
		return err
	}

	cl := NewOverlayClient(conn)

	ctx := metadata.NewOutgoingContext(context.Background(), hd)

	client, err := cl.Relay(ctx)
	if err != nil {
		return err
	}

	rtingAny, err := sender.encoder.PackAny(rting)
	if err != nil {
		return err
	}

	client.Send(&Envelope{Message: &Message{Payload: rtingAny}})

	o.setupStream(client, sender, relay)

	return nil
}

func (o overlay) setupStream(stream relayer, sender *sender, addr mino.Address) {
	// Relay sender for that connection.
	ch := make(chan *Envelope)
	sender.clients[addr] = ch

	go func() {
		for {
			env := <-ch
			err := stream.Send(env)
			if err == io.EOF {
				return
			}

			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}
		}
	}()

	// Relay listener for that connection.
	go func() {
		for {
			envelope, err := stream.Recv()
			if err == io.EOF {
				return
			}

			if err != nil {
				fabric.Logger.Err(err).Send()
				return
			}

			sender.sendEnvelope(envelope, nil)
		}
	}()
}

func makeCertificate() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("Couldn't generate the private key: %+v", err)
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
		return nil, xerrors.Errorf("Couldn't create the certificate: %+v", err)
	}

	cert, err := x509.ParseCertificate(buf)
	if err != nil {
		return nil, xerrors.Errorf("Couldn't parse the certificate: %+v", err)
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

// FromAddress creates a gRPC connection from the server to the client.
func (f DefaultConnectionFactory) FromAddress(addr mino.Address) (grpc.ClientConnInterface, error) {
	clientPubCertItf, found := f.certs.Load(addr)
	if !found {
		return nil, xerrors.Errorf("public certificate for '%v' not found in %v",
			addr, f.certs)
	}

	clientPubCert, ok := clientPubCertItf.(*tls.Certificate)
	if !ok {
		return nil, xerrors.Errorf("couldn't find server <%v> certificate", addr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(clientPubCert.Leaf)

	me, found := f.certs.Load(f.me)
	if !found {
		return nil, xerrors.Errorf("couldn't find server <%v> certificate", f.me)
	}

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*me.(*tls.Certificate)},
		RootCAs:      pool,
	})

	netAddr, ok := addr.(address)
	if !ok {
		return nil, xerrors.Errorf("invalid address type '%T'", addr)
	}

	// Connecting using TLS and the distant server certificate as the root.
	conn, err := grpc.Dial(netAddr.host, grpc.WithTransportCredentials(ta),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: defaultMinConnectTimeout,
		}))
	if err != nil {
		return nil, xerrors.Errorf("failed to create a dial connection: %v", err)
	}

	return conn, nil
}
