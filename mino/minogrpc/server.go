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
	headerStreamIDKey   = "streamid"
	certificateDuration = time.Hour * 24 * 180
)

var (
	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 7 * time.Second
)

type overlayServer struct {
	overlay

	endpoints map[string]*Endpoint
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

	endpoint, found := o.endpoints[uri]
	if !found {
		return nil, xerrors.Errorf("handler '%s' is not registered", uri)
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

	endpoint, found := o.endpoints[uri]
	if !found {
		return xerrors.Errorf("handler '%s' is not registered", uri)
	}

	gateway := gatewayFromContext(stream.Context(), o.addrFactory)
	if gateway == nil {
		return xerrors.Errorf("failed to get gateway, result is nil")
	}

	streamID := streamIDFromContext(stream.Context())
	if streamID == "" {
		return xerrors.Errorf("failed to get streamID, result is empty")
	}

	first := false

	endpoint.Lock()
	session, found := endpoint.streams[streamID]

	if !found {
		first = true

		errs := make(chan error, 1)
		receiver := receiver{
			context:        o.context,
			factory:        endpoint.Factory,
			addressFactory: o.addrFactory,
			errs:           errs,
			queue:          newNonBlockingQueue(),
			logger: dela.Logger.With().Str("addr", o.me.String()).Logger().
				With().Str("streamID", streamID).Logger(),
		}

		sender := sender{
			me:       o.me,
			receiver: receiver,
			traffic:  o.traffic,

			router:      o.router,
			connFactory: o.connFactory,
			uri:         uri,
			gateway:     gateway,
			streamID:    streamID,

			// used to create and close relays
			lock: new(sync.Mutex),

			// used to notify when the context is done
			done: make(chan struct{}),

			connections: make(map[mino.Address]safeRelay),

			relaysWait: new(sync.WaitGroup),
		}

		session = &StreamSession{
			sender:   sender,
			receiver: receiver,
		}

		endpoint.streams[streamID] = session

		sender.connections[gateway] = &passiveConn{streamConn{client: stream}}

	}

	endpoint.Unlock()

	mebuf, err := o.me.MarshalText()
	if err != nil {
		return xerrors.Errorf("failed to marshal my address: %v", err)
	}

	relayCtx := metadata.NewOutgoingContext(stream.Context(), metadata.Pairs(
		headerURIKey, uri, headerGatewayKey, string(mebuf), headerStreamIDKey,
		streamID))

	go session.sender.listenStream(relayCtx, stream, gateway)

	if first {
		err := endpoint.Handler.Stream(session.sender, session.receiver)
		if err != nil {
			return xerrors.Errorf("handler failed to process: %v", err)
		}

		// The participant is done but waits for the protocol to end.
		<-stream.Context().Done()

		endpoint.Lock()
		close(session.sender.done)

		// be sure no relays are still listening before closing the connections,
		// otherwise the listen function of such relay would return an error.
		session.sender.relaysWait.Wait()
		session.sender.closeRelays()
		session.sender.receiver.logger.Trace().Msg("connections closed")

		delete(endpoint.streams, streamID)
		endpoint.Unlock()

		return nil
	}

	// The participant is done but waits for the protocol to end.
	<-stream.Context().Done()

	return nil
}

func (s sender) closeRelays() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for addr, streamConn := range s.connections {
		err := streamConn.close()
		if err != nil {
			s.receiver.logger.Warn().Msgf("failed to close relay '%s': %v", addr, err)
		} else {
			s.traffic.addEvent("close", s.me, addr)
		}
	}
}

// sendPacket creates the relays if needed and sends the packets accordingly.
func (s sender) sendPacket(ctx context.Context, proto *Packet) error {

	packets, err := s.router.Forward(s.me, proto.Serialized, s.receiver.context,
		s.receiver.addressFactory)
	if err != nil {
		return xerrors.Errorf("failed to route packet: %v", err)
	}

	for to, packet := range packets {

		// The router should send nil when it doesn't know the 'from' and the
		// 'to'.
		if to == nil {
			to = newRootAddress()
		}

		if to.Equal(s.me) {
			s.receiver.appendMessage(packet)
			continue
		}

		// If we are not the orchestrator and we must send a message to it, our
		// only option is to send the message back to our gateway.
		if to.Equal(newRootAddress()) {
			to = s.gateway
		}

		// If not already done, setup the connection to this address
		err := s.SetupRelay(ctx, to)
		if err != nil {
			return xerrors.Errorf("failed to setup relay to '%s': %v",
				to, err)
		}

		buf, err := packet.Serialize(s.receiver.context)
		if err != nil {
			return xerrors.Errorf("failed to serialize packet: %v", err)
		}

		proto := &Packet{
			Serialized: buf,
		}

		s.traffic.logSend(ctx, s.me, to, packet)

		err = s.send(to, proto)
		if err != nil {
			s.receiver.logger.Warn().Msgf("failed to send to relay '%s': %v",
				to, err)
		}
	}

	return nil
}

func (s sender) SetupRelay(ctx context.Context, addr mino.Address) error {

	s.lock.Lock()
	defer s.lock.Unlock()

	_, found := s.connections[addr]
	if !found {
		s.receiver.logger.Trace().Msgf("opening a relay to %s", addr)

		conn, err := s.connFactory.FromAddress(addr)
		if err != nil {
			return xerrors.Errorf("failed to create addr: %v", err)
		}

		cl := NewOverlayClient(conn)

		r, err := cl.Stream(ctx, grpc.WaitForReady(false))
		if err != nil {
			// TODO: this error is actually never seen by the user
			s.receiver.logger.Fatal().Msgf(
				"failed to call stream for relay '%s': %v", addr, err)
			return xerrors.Errorf("'%s' failed to call stream for "+
				"relay '%s': %v", s.me, addr, err)
		}

		relay := &streamConn{client: r, conn: conn}

		s.traffic.addEvent("open", s.me, addr)

		s.connections[addr] = relay
		go s.listenStream(ctx, relay, addr)
	}

	return nil
}

// listenStream listens for new messages that would come from a stream that we
// set up and either notify us, or relay the message. This function is blocking.
func (s sender) listenStream(streamCtx context.Context, stream relayable,
	distantAddr mino.Address) {

	s.relaysWait.Add(1)
	defer s.relaysWait.Done()

	for {
		select {
		// Allows us to perform an early stopping in case the contex is already
		// done.
		case <-streamCtx.Done():
			return
		default:
			proto, err := stream.Recv()
			if err == io.EOF {
				return
			}
			status, ok := status.FromError(err)
			if ok && status.Code() == codes.Canceled {
				return
			}
			if err != nil {
				// TODO: this error is actually never seen by the user
				s.receiver.logger.Warn().Msgf("stream failed to receive: %v", err)
				s.receiver.errs <- xerrors.Errorf("failed to receive: %v", err)
				return
			}

			s.traffic.logRcv(stream.Context(), distantAddr, s.me)

			err = s.sendPacket(streamCtx, proto)
			if err != nil {
				s.receiver.errs <- xerrors.Errorf("failed to send to "+
					"dispatched relays: %v", err)
				return
			}
		}
	}
}

// relayable describes the basic primitives to use a stream
type relayable interface {
	Context() context.Context
	Send(*Packet) error
	Recv() (*Packet, error)
}

// safeRelay is a wrapper for a relayable that must have a thread-safe Recv()
// method and a close method
type safeRelay interface {
	relayable
	close() error
}

// streamConn offers a safe way to use a stream connection, with a thread-safe
// Recv() function.
//
// - implements safeRelay
type streamConn struct {
	client relayable
	lock   sync.Mutex
	conn   grpc.ClientConnInterface
}

// Context implements relayable
func (r *streamConn) Context() context.Context {
	return r.client.Context()
}

// Send implements relayable
func (r *streamConn) Send(e *Packet) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.client.Send(e)
}

// Recv implements relayable
func (r *streamConn) Recv() (*Packet, error) {
	return r.client.Recv()
}

// close implements safeRelay
func (r *streamConn) close() error {
	conn, ok := r.conn.(*grpc.ClientConn)
	if !ok {
		return xerrors.Errorf("expected to have '*grpc.ClientConn', got '%T'", r.conn)
	}

	err := conn.Close()
	if err != nil {
		return xerrors.Errorf("failed to close connection: %v", err)
	}

	return nil
}

// passiveConn is used for the server stream, which can't be closed.
//
// - implements safeRelay
type passiveConn struct {
	streamConn
}

// close implements safeRelay
func (r *passiveConn) close() error {
	return nil
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

func newOverlay(me mino.Address, router router.Router,
	addrFactory mino.AddressFactory, ctx serde.Context) (overlay, error) {

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

func streamIDFromContext(ctx context.Context) string {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	streamID := headers[headerStreamIDKey]
	if len(streamID) == 0 {
		return ""
	}

	return streamID[0]
}
