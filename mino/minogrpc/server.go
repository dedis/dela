package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const (
	// headerURIKey is the key used in rpc header to pass the handler URI
	headerURIKey      = "apiuri"
	headerStreamIDKey = "streamid"
	headerGatewayKey  = "gateway"
	headerAddressKey  = "addr"

	certificateDuration = time.Hour * 24 * 180
)

var (
	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 7 * time.Second
)

type overlayServer struct {
	*overlay

	endpoints map[string]*Endpoint
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
// participant.
func (o overlayServer) Share(ctx context.Context, msg *ptypes.Certificate) (*ptypes.CertificateAck, error) {
	// TODO: verify the validity of the certificate by connecting to the distant
	// node but that requires a protection against malicious share.

	from := o.addrFactory.FromText(msg.GetAddress())

	cert, err := x509.ParseCertificate(msg.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't parse certificate: %v", err)
	}

	o.certs.Store(from, &tls.Certificate{Leaf: cert})

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

	uri, streamID, gateway := readHeaders(headers)
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
		headerGatewayKey, o.meStr)

	var relay session.Relay
	var conn grpc.ClientConnInterface
	if isRoot {
		// The relay back to the orchestrator is using the stream as this is the
		// only way to get back to it. Fortunately, if the stream succeeds, it
		// means the packet arrived.
		relay = session.NewStreamRelay(newRootAddress(), stream, o.context)
	} else {
		gw := o.addrFactory.FromText([]byte(gateway))

		conn, err = o.connMgr.Acquire(gw)
		if err != nil {
			return xerrors.Errorf("gateway connection failed: %v", err)
		}

		defer o.connMgr.Release(gw)

		relay = session.NewRelay(stream, gw, o.context, conn, md)
	}

	sess := session.NewSession(
		md,
		relay,
		o.me,
		table,
		endpoint.Factory,
		o.router.GetPacketFactory(),
		o.context,
		o.connMgr,
	)

	// TODO: support multiple parents and clean the stream.
	endpoint.Lock()
	endpoint.streams[streamID] = sess
	endpoint.Unlock()

	o.closer.Add(1)

	go func() {
		sess.Listen(stream)
		o.closer.Done()
	}()

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

func (o *overlayServer) tableFromHeaders(h metadata.MD) (router.RoutingTable, bool, error) {
	values := h.Get(session.HandshakeKey)
	if len(values) != 0 {
		hs, err := o.overlay.router.GetHandshakeFactory().HandshakeOf(o.context, []byte(values[0]))
		if err != nil {
			return nil, false, xerrors.Errorf("malformed handshake: %v", err)
		}

		table, err := o.router.TableOf(hs)
		if err != nil {
			return nil, false, xerrors.Errorf("invalid handshake: %v", err)
		}

		return table, false, nil
	}

	values = h.Get(headerAddressKey)
	if len(values) == 0 {
		return nil, false, xerrors.New("headers are empty")
	}

	addrs := make([]mino.Address, len(values))
	for i, addr := range values {
		addrs[i] = o.addrFactory.FromText([]byte(addr))
	}

	table, err := o.router.New(mino.NewAddresses(addrs...))
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

	uri, streamID, gateway := readHeaders(headers)

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
	me          mino.Address
	certs       certs.Storage
	tokens      tokens.Holder
	router      router.Router
	connMgr     session.ConnectionManager
	addrFactory mino.AddressFactory

	// Keep a text marshalled value for the overlay address so that it's not
	// calculated for each request.
	meStr string
}

func newOverlay(me mino.Address, router router.Router,
	addrFactory mino.AddressFactory, ctx serde.Context) (*overlay, error) {

	cert, err := makeCertificate()
	if err != nil {
		return nil, xerrors.Errorf("failed to make certificate: %v", err)
	}

	meBytes, err := me.MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal address: %v", err)
	}

	certs := certs.NewInMemoryStore()
	certs.Store(me, cert)

	o := &overlay{
		closer:      new(sync.WaitGroup),
		context:     ctx,
		me:          me,
		meStr:       string(meBytes),
		tokens:      tokens.NewInMemoryHolder(),
		certs:       certs,
		router:      router,
		connMgr:     newConnManager(me, certs),
		addrFactory: addrFactory,
	}

	return o, nil
}

// GetCertificate returns the certificate of the overlay.
func (o *overlay) GetCertificate() *tls.Certificate {
	me := o.certs.Load(o.me)
	if me == nil {
		// This should never happen and it will panic if it does as this will
		// provoke several issues later on.
		panic("certificate of the overlay must be populated")
	}

	return me
}

// AddCertificateStore returns the certificate store.
func (o *overlay) GetCertificateStore() certs.Storage {
	return o.certs
}

// Join sends a join request to a distant node with token generated beforehands
// by the later.
func (o *overlay) Join(addr, token string, certHash []byte) error {
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

	conn, err := o.connMgr.Acquire(target)
	if err != nil {
		return xerrors.Errorf("couldn't open connection: %v", err)
	}

	defer o.connMgr.Release(target)

	client := ptypes.NewOverlayClient(conn)

	req := &ptypes.JoinRequest{
		Token: token,
		Certificate: &ptypes.Certificate{
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

// ConnManager is a manager to dial and close connections depending how the
// usage.
//
// - implements session.ConnectionManager
type connManager struct {
	sync.Mutex
	certs    certs.Storage
	me       mino.Address
	counters map[mino.Address]int
	conns    map[mino.Address]*grpc.ClientConn
}

func newConnManager(me mino.Address, certs certs.Storage) *connManager {
	return &connManager{
		certs:    certs,
		me:       me,
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

	clientPubCert := mgr.certs.Load(to)
	if clientPubCert == nil {
		return nil, xerrors.Errorf("certificate for '%v' not found", to)
	}

	pool := x509.NewCertPool()
	pool.AddCert(clientPubCert.Leaf)

	me := mgr.certs.Load(mgr.me)
	if me == nil {
		return nil, xerrors.Errorf("couldn't find server '%v' certificate", mgr.me)
	}

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*me},
		RootCAs:      pool,
	})

	netAddr, ok := to.(address)
	if !ok {
		return nil, xerrors.Errorf("invalid address type '%T'", to)
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

	mgr.conns[to] = conn
	mgr.counters[to] = 1

	return conn, nil
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
				Stringer("from", mgr.me).
				Int("length", len(mgr.conns)).
				Msg("connection closed")

			return
		}

		mgr.counters[to]--
	}
}

func readHeaders(md metadata.MD) (uri string, streamID string, gw string) {
	uri = getOrEmpty(md, headerURIKey)
	streamID = getOrEmpty(md, headerStreamIDKey)
	gw = getOrEmpty(md, headerGatewayKey)

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
