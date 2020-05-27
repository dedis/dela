package minogrpc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/routing"
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

func (o overlayServer) Join(ctx context.Context, req *JoinRequest) (*JoinResponse, error) {
	// 1. Check validity of the token.
	if !o.tokens.Verify(req.Token) {
		return nil, xerrors.New("token is invalid")
	}

	from := o.routingFactory.GetAddressFactory().FromText(req.Certificate.GetAddress())

	leaf, err := x509.ParseCertificate(req.Certificate.GetValue())
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{Leaf: leaf}

	// 2. Share certificates to current participants.
	peers := make([]*Certificate, 0)
	res := make(chan error, 1)

	o.certs.Range(func(addr mino.Address, cert *tls.Certificate) bool {
		text, err := addr.MarshalText()
		if err != nil {
			return false
		}

		msg := &Certificate{Address: text, Value: req.GetCertificate().GetValue()}

		peers = append(peers, msg)

		go func() {
			conn, err := o.connFactory.FromAddress(addr)
			if err != nil {
				res <- err
				return
			}

			client := NewOverlayClient(conn)

			_, err = client.Share(ctx, msg)
			if err != nil {
				res <- err
				return
			}

			res <- nil
		}()

		return true
	})

	ack := 0
	for ack < len(peers) {
		err := <-res
		if err != nil {
			return nil, err
		}

		ack++
	}

	o.certs.Store(from, cert)

	// 3. Return the set of known certificates.
	return &JoinResponse{Peers: peers}, nil
}

func (o overlayServer) Share(ctx context.Context, msg *Certificate) (*CertificateAck, error) {
	// TODO: store and verify

	return &CertificateAck{}, nil
}

// Call implements minogrpc.OverlayClient. It processes the request with the
// targeted handler if it exists, otherwise it returns an error.
func (o overlayServer) Call(ctx context.Context, msg *Message) (*Message, error) {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	uri := uriFromContext(ctx)

	// If several are provided, only the first one is taken in account.
	handler, ok := o.handlers[uri]
	if !ok {
		return nil, xerrors.Errorf("handler '%s' is not registered", uri)
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

// Stream implements minogrpc.OverlayClient. It opens streams according to the
// routing and transmits the message according to their recipient.
func (o overlayServer) Stream(stream Overlay_StreamServer) error {
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
	certs          CertificateStore
	tokens         *TokenHolder
	routingFactory routing.Factory
	connFactory    ConnectionFactory
	traffic        *traffic
}

func newOverlay(me mino.Address, rf routing.Factory) (overlay, error) {
	cert, err := makeCertificate()
	if err != nil {
		return overlay{}, xerrors.Errorf("failed to make certificate: %v", err)
	}

	certs := NewInMemoryCertStore()
	certs.Store(me, cert)

	o := overlay{
		encoder:        encoding.NewProtoEncoder(),
		me:             me,
		tokens:         NewTokenHolder(),
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
	me := o.certs.Load(o.me)
	if me == nil {
		// This should never happen and it will panic if it does as this will
		// provoke several issues later on.
		panic("certificate of the overlay must be populated")
	}

	return me
}

// AddCertificateStore returns the certificate store.
func (o overlay) GetCertificateStore() CertificateStore {
	return o.certs
}

// Join sends a join request to a distant node with token generated beforehands
// by the later.
func (o overlay) Join(addr, token string, certHash []byte) error {
	meCert := o.GetCertificate()

	meAddr, err := o.me.MarshalText()
	if err != nil {
		return err
	}

	target := o.routingFactory.GetAddressFactory().FromText([]byte(addr))

	// Fetch the certificate of the node we want to join. The hash is used to
	// ensure that we get the right certificate.
	err = o.certs.Fetch(target, certHash)
	if err != nil {
		return err
	}

	conn, err := o.connFactory.FromAddress(target)
	if err != nil {
		return err
	}

	client := NewOverlayClient(conn)

	req := &JoinRequest{
		Token: token,
		Certificate: &Certificate{
			Address: meAddr,
			Value:   meCert.Leaf.Raw,
		},
	}

	resp, err := client.Join(context.Background(), req)
	if err != nil {
		return err
	}

	// Update the certificate store with the response from the node we just
	// joined. That will allow the node to communicate with the network.
	for _, raw := range resp.Peers {
		from := o.routingFactory.GetAddressFactory().FromText(raw.GetAddress())

		leaf, err := x509.ParseCertificate(raw.GetValue())
		if err != nil {
			return xerrors.Errorf("couldn't parse certificate: %v", err)
		}

		o.certs.Store(from, &tls.Certificate{Leaf: leaf})
	}

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
		dela.Logger.Trace().
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

	client, err := cl.Stream(ctx)
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

func (o overlay) setupStream(stream relayer, sender *sender, receiver *receiver,
	addr mino.Address) {

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
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{},
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
	certs CertificateStore
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

// CertificateStore is an interface to manage the certificates of a server.
type CertificateStore interface {
	Store(mino.Address, *tls.Certificate)
	Load(mino.Address) *tls.Certificate
	Delete(mino.Address)
	Range(func(addr mino.Address, cert *tls.Certificate) bool)
	Fetch(mino.Address, []byte) error
	Hash(*tls.Certificate) ([]byte, error)
}

// InMemoryCertStore is a certificate store that keeps the certificates in
// memory only which means it does not persist.
type InMemoryCertStore struct {
	certs *sync.Map
}

// NewInMemoryCertStore creates a new empty certificate store.
func NewInMemoryCertStore() *InMemoryCertStore {
	return &InMemoryCertStore{
		certs: &sync.Map{},
	}
}

// Store implements minogrpc.Store.
func (s *InMemoryCertStore) Store(addr mino.Address, cert *tls.Certificate) {
	s.certs.Store(addr, cert)
}

// Load implements minogrpc.CertificateStore.
func (s *InMemoryCertStore) Load(addr mino.Address) *tls.Certificate {
	val, found := s.certs.Load(addr)
	if !found {
		return nil
	}
	return val.(*tls.Certificate)
}

// Delete implements minogrpc.CertificateStore.
func (s *InMemoryCertStore) Delete(addr mino.Address) {
	s.certs.Delete(addr)
}

// Range implements minogrpc.CertificateStore.
func (s *InMemoryCertStore) Range(fn func(addr mino.Address, cert *tls.Certificate) bool) {
	s.certs.Range(func(key, value interface{}) bool {
		return fn(key.(mino.Address), value.(*tls.Certificate))
	})
}

// Fetch implements minogrpc.CertificateStore.
func (s *InMemoryCertStore) Fetch(addr mino.Address, hash []byte) error {
	netAddr, ok := addr.(address)
	if !ok {
		return xerrors.Errorf("invalid address type '%T'", addr)
	}

	certCh := make(chan *tls.Certificate)

	ta := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(raw [][]byte, chains [][]*x509.Certificate) error {
			defer close(certCh)

			if len(raw) > 0 {
				leaf, err := x509.ParseCertificate(raw[0])
				if err != nil {
					return err
				}

				certCh <- &tls.Certificate{Leaf: leaf}
			}
			return nil
		},
	})

	// This connection will be used to fetch the certificate of the server and
	// to verify that it matches the expected hash.
	conn, err := grpc.Dial(netAddr.GetDialAddress(), grpc.WithTransportCredentials(ta))
	if err != nil {
		return xerrors.Errorf("failed to dial: %v", err)
	}

	cert := <-certCh
	conn.Close()

	if cert == nil {
		return xerrors.Errorf("failed to get cert")
	}

	digest, err := s.Hash(cert)
	if err != nil {
		return err
	}

	if !bytes.Equal(digest, hash) {
		return xerrors.Errorf("invalid check sum")
	}

	s.certs.Store(addr, cert)

	return nil
}

// Hash implements minogrpc.CertificateStore.
func (s *InMemoryCertStore) Hash(cert *tls.Certificate) ([]byte, error) {
	h := sha256.New()
	h.Write(cert.Leaf.Raw)

	return h.Sum(nil), nil
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
