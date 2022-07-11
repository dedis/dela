// This file contains the implementation of the inner overlay of the minogrpc
// instance which processes the requests from the gRPC server.
//
// Dcoumentation Last Review: 07.10.2020
//

package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"net/url"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/internal/traffic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	otgrpc "github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
)

const (
	// headerURIKey is the key used in rpc header to pass the handler URI
	headerURIKey      = "apiuri"
	headerStreamIDKey = "streamid"
	headerGatewayKey  = "gateway"
	// headerAddressKey is the key of the header that contains the list of
	// addresses that will allow a node to create a routing table.
	headerAddressKey = "addr"

	certificateDuration = time.Hour * 24 * 180

	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 10 * time.Second
)

var getTracerForAddr = tracing.GetTracerForAddr

type overlayServer struct {
	*overlay
	ptypes.UnimplementedOverlayServer

	endpoints map[string]*Endpoint
	notifier  traffic.Notifier
}

// Join implements ptypes.OverlayServer. It processes the request by checking
// the validity of the token and if it is accepted, by sending the certificate
// to the known peers. It finally returns the certificates to the caller.
func (o overlayServer) Join(ctx context.Context, req *ptypes.JoinRequest) (*ptypes.JoinResponse, error) {
	// 1. Check validity of the token.
	if !o.tokens.Verify(req.Token) {
		return nil, xerrors.Errorf("token '%s' is invalid", req.Token)
	}

	dela.Logger.Debug().
		Str("from", string(req.GetCertificate().GetAddress())).
		Msg("valid token received")

	// 2. Share certificates to current participants.
	list := make(map[mino.Address][]byte)
	o.certs.Range(func(addr mino.Address, cert *tls.Certificate) bool {
		list[addr] = cert.Leaf.Raw
		return true
	})

	peers := make([]*ptypes.Certificate, 0, len(list))
	res := make(chan error, 1)

	for to, cert := range list {
		text, err := to.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		msg := &ptypes.Certificate{Address: text, Value: cert}

		// Prepare the list of known certificates to send back to the new node.
		peers = append(peers, msg)

		// Share the new node certificate with existing peers.
		go func(to mino.Address) {
			conn, err := o.connMgr.Acquire(to)
			if err != nil {
				res <- xerrors.Errorf("couldn't open connection: %v", err)
				return
			}

			defer o.connMgr.Release(to)

			client := ptypes.NewOverlayClient(conn)

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
	return &ptypes.JoinResponse{Peers: peers}, nil
}

// Share implements ptypes.OverlayServer. It accepts a certificate from a
// participant only if it is valid from the address it claims to be.
func (o overlayServer) Share(ctx context.Context, msg *ptypes.Certificate) (*ptypes.CertificateAck, error) {
	from := o.addrFactory.FromText(msg.GetAddress()).(session.Address)

	cert, err := x509.ParseCertificate(msg.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't parse certificate: %v", err)
	}

	// Make sure the certificate is valid for the public key provided.
	err = cert.CheckSignatureFrom(cert)
	if err != nil {
		return nil, xerrors.Errorf("invalid certificate signature: %v", err)
	}

	hostname, err := from.GetHostname()
	if err != nil {
		return nil, xerrors.Errorf("malformed address: %v", err)
	}

	err = cert.VerifyHostname(hostname)
	if err != nil {
		return nil, xerrors.Errorf("invalid hostname: %v", err)
	}

	o.certs.Store(from, &tls.Certificate{
		Certificate: [][]byte{msg.GetValue()},
		Leaf:        cert,
	})

	return &ptypes.CertificateAck{}, nil
}

// Call implements minogrpc.OverlayServer. It processes the request with the
// targeted handler if it exists, otherwise it returns an error.
func (o overlayServer) Call(ctx context.Context, msg *ptypes.Message) (*ptypes.Message, error) {
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
		return &ptypes.Message{}, nil
	}

	res, err := result.Serialize(o.context)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize result: %v", err)
	}

	return &ptypes.Message{Payload: res}, nil
}

// Stream implements ptypes.OverlayServer. It can be called in two different
// situations: 1. the node is the very first contacted by the orchestrator and
// will then lead the protocol, or 2. the node is part of the protocol.
//
// A stream is always built with the orchestrator as the original caller that
// contacts one of the participant. The chosen one will relay the messages to
// and from the orchestrator.
//
// Other participants of the protocol will wake up when receiving a message from
// a parent which will open a stream that will determine when to close. If they
// have to relay a message, they will use a unicast call recursively until the
// message is delivered, or has failed. This allows to return an acknowledgement
// that contains the missing delivered addresses.
func (o *overlayServer) Stream(stream ptypes.Overlay_StreamServer) error {
	o.closer.Add(1)
	defer o.closer.Done()

	headers, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return xerrors.New("missing headers")
	}

	table, isRoot, err := o.tableFromHeaders(headers)
	if err != nil {
		return xerrors.Errorf("routing table: %v", err)
	}

	uri, streamID, gateway, protocol := readHeaders(headers)
	if streamID == "" {
		return xerrors.New("unexpected empty stream ID")
	}

	endpoint, found := o.endpoints[uri]
	if !found {
		return xerrors.Errorf("handler '%s' is not registered", uri)
	}

	md := metadata.Pairs(
		headerURIKey, uri,
		headerStreamIDKey, streamID,
		headerGatewayKey, o.myAddrStr,
		tracing.ProtocolTag, protocol,
	)

	// This lock will make sure that a session is ready before any message
	// forwarded will be received.
	endpoint.Lock()

	sess, initiated := endpoint.streams[streamID]
	if !initiated {
		sess = session.NewSession(
			md,
			o.myAddr,
			endpoint.Factory,
			o.router.GetPacketFactory(),
			o.context,
			o.connMgr,
			o.notifier,
		)

		endpoint.streams[streamID] = sess
	}

	gatewayAddr := o.addrFactory.FromText([]byte(gateway))

	var relay session.Relay
	var conn grpc.ClientConnInterface
	if isRoot {
		// Only the original address is sent to make use of the cached marshaled
		// value, so it needs to be upgraded to an orchestrator address.
		gatewayAddr = session.NewOrchestratorAddress(gatewayAddr)

		// The relay back to the orchestrator is using the stream as this is the
		// only way to get back to it. Fortunately, if the stream succeeds, it
		// means the packet arrived.
		relay = session.NewStreamRelay(gatewayAddr, stream, o.context)
	} else {
		conn, err = o.connMgr.Acquire(gatewayAddr)
		if err != nil {
			endpoint.Unlock()
			return xerrors.Errorf("gateway connection failed: %v", err)
		}

		defer o.connMgr.Release(gatewayAddr)

		relay = session.NewRelay(stream, gatewayAddr, o.context, conn, md)
	}

	o.closer.Add(1)

	ready := make(chan struct{})

	go func() {
		sess.Listen(relay, table, ready)

		o.cleanStream(endpoint, streamID)
		o.closer.Done()
	}()

	// There, it is important to make sure the parent relay is registered in the
	// session before releasing the endpoint.
	<-ready

	endpoint.Unlock()

	// This event sends a confirmation to the parent that the stream is
	// registered and it can send messages to it.
	err = stream.SendHeader(make(metadata.MD))
	if err != nil {
		return xerrors.Errorf("failed to send header: %v", err)
	}

	err = endpoint.Handler.Stream(sess, sess)
	if err != nil {
		return xerrors.Errorf("handler failed to process: %v", err)
	}

	<-stream.Context().Done()

	return nil
}

func (o *overlayServer) cleanStream(endpoint *Endpoint, id string) {
	endpoint.Lock()
	defer endpoint.Unlock()

	// It's important to check the session currently stored as it may be a new
	// one with an active parent, or it might be already cleaned.
	sess, found := endpoint.streams[id]
	if !found {
		return
	}

	if sess.GetNumParents() > 0 {
		return
	}

	delete(endpoint.streams, id)

	sess.Close()

	dela.Logger.Trace().
		Str("id", id).
		Msg("stream has been cleaned")
}

func (o *overlayServer) tableFromHeaders(h metadata.MD) (router.RoutingTable, bool, error) {
	values := h.Get(session.HandshakeKey)
	if len(values) != 0 {
		hs, err := o.overlay.router.GetHandshakeFactory().HandshakeOf(o.context, []byte(values[0]))
		if err != nil {
			return nil, false, xerrors.Errorf("malformed handshake: %v", err)
		}

		table, err := o.router.GenerateTableFrom(hs)
		if err != nil {
			return nil, false, xerrors.Errorf("invalid handshake: %v", err)
		}

		return table, false, nil
	}

	values = h.Get(headerAddressKey)

	addrs := make([]mino.Address, len(values))
	for i, addr := range values {
		addrs[i] = o.addrFactory.FromText([]byte(addr))
	}

	table, err := o.router.New(mino.NewAddresses(addrs...), o.myAddr)
	if err != nil {
		return nil, true, xerrors.Errorf("failed to create: %v", err)
	}

	return table, true, nil
}

// Forward implements ptypes.OverlayServer. It handles a request to forward a
// packet by sending it to the appropriate session.
func (o *overlayServer) Forward(ctx context.Context, p *ptypes.Packet) (*ptypes.Ack, error) {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, xerrors.New("no header in the context")
	}

	uri, streamID, gateway, _ := readHeaders(headers)

	endpoint, found := o.endpoints[uri]
	if !found {
		return nil, xerrors.Errorf("handler '%s' is not registered", uri)
	}

	endpoint.RLock()
	sess, ok := endpoint.streams[streamID]
	endpoint.RUnlock()

	if !ok {
		return nil, xerrors.Errorf("no stream '%s' found", streamID)
	}

	from := o.addrFactory.FromText([]byte(gateway))

	return sess.RecvPacket(from, p)
}

type overlay struct {
	closer      *sync.WaitGroup
	context     serde.Context
	myAddr      session.Address
	certs       certs.Storage
	tokens      tokens.Holder
	router      router.Router
	connMgr     session.ConnectionManager
	addrFactory mino.AddressFactory

	// secret and public are the key pair that has generated the server
	// certificate.
	secret interface{}
	public interface{}

	// Keep a text marshalled value for the overlay address so that it's not
	// calculated for each request.
	myAddrStr string
}

func newOverlay(tmpl minoTemplate) (*overlay, error) {
	// session.Address never returns an error
	myAddrBuf, _ := tmpl.myAddr.MarshalText()

	if tmpl.secret == nil || tmpl.public == nil {
		priv, err := ecdsa.GenerateKey(tmpl.curve, tmpl.random)
		if err != nil {
			return nil, xerrors.Errorf("cert private key: %v", err)
		}

		tmpl.secret = priv
		tmpl.public = priv.Public()
	}

	o := &overlay{
		closer:      new(sync.WaitGroup),
		context:     json.NewContext(),
		myAddr:      tmpl.myAddr,
		myAddrStr:   string(myAddrBuf),
		tokens:      tokens.NewInMemoryHolder(),
		certs:       tmpl.certs,
		router:      tmpl.router,
		connMgr:     newConnManager(tmpl.myAddr, tmpl.certs),
		addrFactory: tmpl.fac,
		secret:      tmpl.secret,
		public:      tmpl.public,
	}

	cert, err := o.certs.Load(o.myAddr)
	if err != nil {
		return nil, xerrors.Errorf("while loading cert: %v", err)
	}

	if cert == nil {
		err = o.makeCertificate()
		if err != nil {
			return nil, xerrors.Errorf("certificate failed: %v", err)
		}
	}

	return o, nil
}

// GetCertificate returns the certificate of the overlay with its private key
// set.
func (o *overlay) GetCertificate() *tls.Certificate {
	me, err := o.certs.Load(o.myAddr)
	if err != nil {
		// An error when getting the certificate of the server is caused by the
		// underlying storage, and that should never happen in healthy
		// environment.
		panic(xerrors.Errorf("certificate of the overlay is inaccessible: %v", err))
	}
	if me == nil {
		// This should never happen and it will panic if it does as this will
		// provoke several issues later on.
		panic("certificate of the overlay must be populated")
	}

	me.PrivateKey = o.secret

	return me
}

// GetCertificateStore returns the certificate store.
func (o *overlay) GetCertificateStore() certs.Storage {
	return o.certs
}

// Join sends a join request to a distant node with token generated beforehands
// by the later.
func (o *overlay) Join(addr *url.URL, token string, certHash []byte) error {

	target := session.NewAddress(addr.Host + addr.Path)

	meCert := o.GetCertificate()

	// Fetch the certificate of the node we want to join. The hash is used to
	// ensure that we get the right certificate.
	err := o.certs.Fetch(target, certHash)
	if err != nil {
		return xerrors.Errorf("couldn't fetch distant certificate: %v", err)
	}

	conn, err := o.connMgr.Acquire(target)
	if err != nil {
		return xerrors.Errorf("couldn't open connection: %v", err)
	}

	defer o.connMgr.Release(target)

	client := ptypes.NewOverlayClient(conn)

	req := &ptypes.JoinRequest{
		Token: token,
		Certificate: &ptypes.Certificate{
			Address: []byte(o.myAddrStr),
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

func (o *overlay) makeCertificate() error {
	var ips []net.IP
	var dnsNames []string

	hostname, err := o.myAddr.GetHostname()
	if err != nil {
		return xerrors.Errorf("failed to get hostname: %v", err)
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		ips = []net.IP{ip}
	} else {
		dnsNames = []string{hostname}
	}

	dela.Logger.Info().Str("hostname", hostname).
		Strs("dnsNames", dnsNames).
		Msgf("creating certificate: ips: %v", ips)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		IPAddresses:  ips,
		DNSNames:     dnsNames,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(certificateDuration),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		MaxPathLen:            1,
		IsCA:                  true,
	}

	buf, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, o.public, o.secret)
	if err != nil {
		return xerrors.Errorf("while creating: %+v", err)
	}

	leaf, err := x509.ParseCertificate(buf)
	if err != nil {
		return xerrors.Errorf("couldn't parse the certificate: %v", err)
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{buf},
		PrivateKey:  o.secret,
		Leaf:        leaf,
	}

	err = o.certs.Store(o.myAddr, cert)
	if err != nil {
		return xerrors.Errorf("while storing: %v", err)
	}

	return nil
}

// ConnManager is a manager to dial and close connections depending on the
// usage.
//
// - implements session.ConnectionManager
type connManager struct {
	sync.Mutex
	certs    certs.Storage
	myAddr   mino.Address
	counters map[mino.Address]int
	conns    map[mino.Address]*grpc.ClientConn
}

func newConnManager(myAddr mino.Address, certs certs.Storage) *connManager {
	return &connManager{
		certs:    certs,
		myAddr:   myAddr,
		counters: make(map[mino.Address]int),
		conns:    make(map[mino.Address]*grpc.ClientConn),
	}
}

// Len implements session.ConnectionManager. It returns the number of active
// connections in the manager.
func (mgr *connManager) Len() int {
	mgr.Lock()
	defer mgr.Unlock()

	return len(mgr.conns)
}

// Acquire implements session.ConnectionManager. It either dials to open the
// connection or returns an existing one for the address.
func (mgr *connManager) Acquire(to mino.Address) (grpc.ClientConnInterface, error) {
	mgr.Lock()
	defer mgr.Unlock()

	conn, ok := mgr.conns[to]
	if ok {
		mgr.counters[to]++
		return conn, nil
	}

	ta, err := mgr.getTransportCredential(to)
	if err != nil {
		return nil, xerrors.Errorf("failed to retrieve transport credential: %v", err)
	}

	netAddr, ok := to.(session.Address)
	if !ok {
		return nil, xerrors.Errorf("invalid address type '%T'", to)
	}

	// Connecting using TLS and the distant server certificate as the root.
	addr := netAddr.GetDialAddress()
	tracer, err := getTracerForAddr(addr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tracer for addr %s: %v", addr, err)
	}

	conn, err = grpc.Dial(addr,
		grpc.WithTransportCredentials(ta),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: defaultMinConnectTimeout,
		}),
		grpc.WithUnaryInterceptor(
			otgrpc.OpenTracingClientInterceptor(tracer, otgrpc.SpanDecorator(decorateClientTrace)),
		),
		grpc.WithStreamInterceptor(
			otgrpc.OpenTracingStreamClientInterceptor(tracer, otgrpc.SpanDecorator(decorateClientTrace)),
		),
	)
	if err != nil {
		return nil, xerrors.Errorf("failed to dial: %v", err)
	}

	mgr.conns[to] = conn
	mgr.counters[to] = 1

	return conn, nil
}

func (mgr *connManager) getTransportCredential(addr mino.Address) (credentials.TransportCredentials, error) {
	clientPubCert, err := mgr.certs.Load(addr)
	if err != nil {
		return nil, xerrors.Errorf("while loading distant cert: %v", err)
	}
	if clientPubCert == nil {
		return nil, xerrors.Errorf("certificate for '%v' not found", addr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(clientPubCert.Leaf)

	me, err := mgr.certs.Load(mgr.myAddr)
	if err != nil {
		return nil, xerrors.Errorf("while loading own cert: %v", err)
	}
	if me == nil {
		return nil, xerrors.Errorf("couldn't find server '%v' certificate", mgr.myAddr)
	}

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*me},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	})

	return ta, nil
}

// Release implements session.ConnectionManager. It closes the connection to the
// address if appropriate.
func (mgr *connManager) Release(to mino.Address) {
	mgr.Lock()
	defer mgr.Unlock()

	count, ok := mgr.counters[to]
	if ok {
		if count <= 1 {
			delete(mgr.counters, to)

			conn := mgr.conns[to]
			delete(mgr.conns, to)

			err := conn.Close()
			dela.Logger.Trace().
				Err(err).
				Stringer("to", to).
				Stringer("from", mgr.myAddr).
				Int("length", len(mgr.conns)).
				Msg("connection closed")

			return
		}

		mgr.counters[to]--
	}
}

func readHeaders(md metadata.MD) (uri string, streamID string, gw string, protocol string) {
	uri = getOrEmpty(md, headerURIKey)
	streamID = getOrEmpty(md, headerStreamIDKey)
	gw = getOrEmpty(md, headerGatewayKey)
	protocol = getOrEmpty(md, tracing.ProtocolTag)

	return
}

func getOrEmpty(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func uriFromContext(ctx context.Context) string {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	apiURI := headers.Get(headerURIKey)
	if len(apiURI) == 0 {
		return ""
	}

	return apiURI[0]
}

// decorateClientTrace adds the protocol tag and the streamID tag to a client
// side trace.
func decorateClientTrace(ctx context.Context, span opentracing.Span, method string,
	req, resp interface{}, grpcError error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return
	}

	protocol := getOrEmpty(md, tracing.ProtocolTag)
	if protocol != "" {
		span.SetTag(tracing.ProtocolTag, protocol)
	}

	streamID := getOrEmpty(md, headerStreamIDKey)
	if streamID != "" {
		span.SetTag(headerStreamIDKey, streamID)
	}
}
