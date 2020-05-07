package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	headerURIKey        = "apiuri"
	certificateDuration = time.Hour * 24 * 180
	// this string is used to identify the orchestrator. We use it as its
	// address.
	orchestratorAddr = "orchestrator_addr"
)

var (
	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 7 * time.Second
)

// Server represents the entity that accepts incoming requests and invoke the
// corresponding RPCs.
type Server struct {
	grpcSrv *grpc.Server

	cert      *tls.Certificate
	addr      mino.Address
	listener  net.Listener
	StartChan chan struct{}

	// neighbours contains the certificate and details about known peers.
	neighbours     map[string]Peer
	routingFactory routing.Factory

	handlers map[string]mino.Handler

	traffic *traffic
}

// Roster is a set of peers that will work together
// to execute protocols.
type Roster []Peer

// Peer is a public identity for a given node.
type Peer struct {
	Address     string
	Certificate *x509.Certificate
}

// RPC represents an RPC that has been registered by a client, which allows
// clients to call an RPC that will execute the provided handler.
//
// - implements mino.RPC
type RPC struct {
	encoder        encoding.ProtoMarshaler
	handler        mino.Handler
	srv            *Server
	uri            string
	routingFactory routing.Factory
}

// Call implements mino.RPC. It calls the RPC on each provided address.
func (rpc *RPC) Call(ctx context.Context, req proto.Message,
	players mino.Players) (<-chan proto.Message, <-chan error) {

	out := make(chan proto.Message, players.Len())
	errs := make(chan error, players.Len())

	m, err := ptypes.MarshalAny(req)
	if err != nil {
		errs <- xerrors.Errorf("failed to marshal msg to any: %v", err)
		return out, errs
	}

	sendMsg := &Envelope{
		Message: m,
	}

	go func() {
		iter := players.AddressIterator()
		for iter.HasNext() {
			addrStr := iter.GetNext().String()

			peer, ok := rpc.srv.neighbours[addrStr]
			if !ok {
				err := xerrors.Errorf("addr '%s' not is our list of neighbours",
					addrStr)
				fabric.Logger.Err(err).Send()
				errs <- err
				continue
			}

			clientConn, err := getConnection(addrStr, peer, *rpc.srv.cert)
			if err != nil {
				errs <- xerrors.Errorf("failed to get client conn for '%s': %v",
					addrStr, err)
				continue
			}
			cl := NewOverlayClient(clientConn)

			header := metadata.New(map[string]string{headerURIKey: rpc.uri})
			newCtx := metadata.NewOutgoingContext(ctx, header)

			callResp, err := cl.Call(newCtx, sendMsg)
			if err != nil {
				errs <- xerrors.Errorf("failed to call client '%s': %v", addrStr, err)
				continue
			}

			resp, err := rpc.encoder.UnmarshalDynamicAny(callResp.Message)
			if err != nil {
				errs <- xerrors.Errorf("couldn't unmarshal message: %v", err)
				continue
			}

			out <- resp
		}

		close(out)
	}()

	return out, errs
}

// Stream implements mino.RPC.
func (rpc RPC) Stream(ctx context.Context,
	players mino.Players) (in mino.Sender, out mino.Receiver) {

	rting, err := rpc.routingFactory.FromIterator(players.AddressIterator())
	if err != nil {
		// TODO better handle this error
		fabric.Logger.Fatal().Msgf("failed to create routing: %v", err)
	}

	// fmt.Print("server tree:")
	// rting.(*routing.TreeRouting).Display(os.Stdout)

	routingProto, err := rting.Pack(rpc.encoder)
	if err != nil {
		// TODO better handle this error
		fabric.Logger.Fatal().Msgf("failed to pack routing: %v", err)
	}

	anyRouting, err := rpc.encoder.MarshalAny(routingProto)
	if err != nil {
		// TODO better handle this error
		fabric.Logger.Fatal().Msgf("failed to encode routing info: %v", err)
	}

	// if every player produces an error the buffer should be large enought so
	// that we are never blocked in the for loop and we can termninate this
	// function.
	errs := make(chan error, players.Len())

	orchSender := &sender{
		encoder: rpc.encoder,
		// This address can't be the server address because we must separate
		// message sent to this server and the ones sent to the sender of this
		// rpc.
		address: address{orchestratorAddr},
		// Participant should contain the stream connection of every child of
		// this node
		name:    "orchestrator",
		srvCert: rpc.srv.cert,
		traffic: rpc.srv.traffic,
		routing: rting,
		// Special case: this is the main orchestrator
		rootAddr: address{orchestratorAddr},
	}

	orchRecv := receiver{
		encoder: rpc.encoder,
		errs:    errs,
		// it is okay to have a blocking chan here because every use of it is in
		// a goroutine, where we don't mind if it blocks.
		in:      make(chan *Envelope),
		name:    "orchestrator",
		traffic: rpc.srv.traffic,
	}

	// Get the root of the tree and creating a stream to it. We also send the
	// routing message, which will trigger the build of the tree.
	// TODO: think how the interface can be changed to allow getting the root
	rootAddr := rting.(*routing.TreeRouting).Root.Addr

	myPeer, ok := rpc.srv.neighbours[rootAddr.String()]
	if !ok {
		err := xerrors.Errorf("my addr '%s' not is our list of neighbours",
			rpc.srv.addr.String())
		fabric.Logger.Fatal().Err(err).Send()
	}

	myConn, err := getConnection(rootAddr.String(), myPeer, *rpc.srv.cert)
	if err != nil {
		fabric.Logger.Err(xerrors.Errorf("failed to get my conn: %v", err)).Send()
	}

	cl := NewOverlayClient(myConn)
	header := metadata.New(map[string]string{headerURIKey: rpc.uri})
	newCtx := metadata.NewOutgoingContext(ctx, header)

	s, err := cl.Stream(newCtx)
	if err != nil {
		err = xerrors.Errorf("failed to get stream for my client '%s': %v",
			rpc.srv.addr.String(), err)
		fabric.Logger.Err(err).Send()
	}

	stream := newSafeOverlayStream(s)

	orchSender.participants.Store(rootAddr.String(), stream)

	// Sending the routing info as first message. In a tree routing topology
	// this means sending this message to the root of the tree, which will then
	// create its direct children that will then do the same.
	stream.Send(&Envelope{
		From:         orchestratorAddr,
		PhysicalFrom: orchestratorAddr,
		To:           []string{rootAddr.String()},
		Message:      anyRouting,
	})

	// Listen on the clients streams and notify the orchestrator or relay
	// messages
	go func(addr mino.Address, stream overlayStream) {
		for {
			addrCopy := address{addr.String()}
			err := listenStream(stream, &orchRecv, orchSender, addrCopy)
			if err == io.EOF {
				<-ctx.Done()
				return
			}
			status, ok := status.FromError(err)
			if ok && err != nil && status.Code() == codes.Canceled {
				<-ctx.Done()
				return
			}
			if err != nil {
				err = xerrors.Errorf("failed to listen stream: %v", err)
				fabric.Logger.Err(err).Send()
				errs <- err
				<-ctx.Done()
				return
			}
		}
	}(address{orchestratorAddr}, stream)

	return orchSender, orchRecv
}

// listenStream reads the client RPC stream and handle the received messages
// accordingly: It formwards messages or notify the orchestrator.
func listenStream(stream overlayStream, orchRecv *receiver,
	orchSender *sender, addr mino.Address) error {

	// This msg.Message should always be an enveloppe
	envelope, err := stream.Recv()
	if err == io.EOF {
		return io.EOF
	}
	status, ok := status.FromError(err)
	if ok && err != nil && status.Code() == codes.Canceled {
		return status.Err()
	}
	if err != nil {
		return xerrors.Errorf("client '%s' failed to "+
			"receive from RPC server: %v", addr.String(), err)
	}

	// In fact the message is received by a client that is handled by an
	// orchestrator. So when someone send a message to the client, it in fact
	// want to reach the orchestrator that holds the client. This is why the
	// "To" attribute is set to orchSender.address
	orchRecv.traffic.logRcv(address{envelope.PhysicalFrom}, orchSender.address,
		envelope, orchRecv.name)

	for _, toSend := range envelope.To {
		// if we receive a message to the orchestrator then we notify the
		// orchestrator receiver by filling orchRecv.in. If this is not the case
		// we relay the message.
		if toSend == orchSender.address.String() {
			orchRecv.in <- envelope
		} else {
			// orchRecv.traffic.logRcvRelay(address{envelope.From}, envelope,
			// 	orchRecv.name)
			fabric.Logger.Trace().Msgf("(orchestrator) relaying message from "+
				"'%s' to '%s'", envelope.From, toSend)

			msg, err := orchRecv.encoder.UnmarshalDynamicAny(envelope.Message)
			if err != nil {
				return xerrors.Errorf("failed to unmarshal enveloppe message: %v", err)
			}

			errChan := orchSender.sendWithFrom(msg,
				address{envelope.From}, address{toSend})
			err, more := <-errChan
			if more {
				return xerrors.Errorf("failed to send relay message: %v", err)
			}
		}
	}
	return nil
}

// NewServer sets up a new server
func NewServer(addr mino.Address, rf routing.Factory) (*Server, error) {
	if addr.String() == "" {
		return nil, xerrors.New("addr.String() should not give an empty string")
	}

	cert, err := makeCertificate()
	if err != nil {
		return nil, xerrors.Errorf("failed to make certificate: %v", err)
	}

	srv := grpc.NewServer(grpc.Creds(credentials.NewServerTLSFromCert(cert)))
	server := &Server{
		grpcSrv:        srv,
		cert:           cert,
		addr:           addr,
		listener:       nil,
		StartChan:      make(chan struct{}),
		neighbours:     make(map[string]Peer),
		handlers:       make(map[string]mino.Handler),
		traffic:        newTraffic(addr.String()),
		routingFactory: rf,
	}

	RegisterOverlayServer(srv, &overlayService{
		encoder:        encoding.NewProtoEncoder(),
		handlers:       server.handlers,
		addr:           address{addr.String()},
		neighbour:      server.neighbours,
		srvCert:        server.cert,
		traffic:        server.traffic,
		routingFactory: rf,
	})

	return server, nil
}

// StartServer makes the server start listening and wait until its started
func (srv *Server) StartServer() {

	go func() {
		err := srv.Serve()
		// TODO: better handle this error
		if err != nil {
			fabric.Logger.Fatal().Msg("failed to start the server " + err.Error())
		}
	}()

	<-srv.StartChan
}

// Serve starts the HTTP server that forwards gRPC calls to the gRPC server
func (srv *Server) Serve() error {
	lis, err := net.Listen("tcp4", srv.addr.String())
	if err != nil {
		return xerrors.Errorf("failed to listen: %v", err)
	}

	srv.listener = lis

	close(srv.StartChan)

	// This call always returns an error
	// err = srv.httpSrv.ServeTLS(lis, "", "")
	err = srv.grpcSrv.Serve(lis)
	if err != nil {
		return xerrors.Errorf("failed to serve, did you forget to call "+
			"server.Stop() or server.GracefulStop()?: %v", err)
	}

	return nil
}

func (srv *Server) addNeighbour(servers ...*Server) {
	for _, server := range servers {
		srv.neighbours[server.addr.String()] = Peer{
			Address:     server.listener.Addr().String(),
			Certificate: server.cert.Leaf,
		}
	}
}

// getConnection creates a gRPC connection from the server to the client.
func getConnection(addr string, peer Peer, cert tls.Certificate) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, xerrors.New("empty address is not allowed")
	}

	pool := x509.NewCertPool()
	pool.AddCert(peer.Certificate)

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	})

	// Connecting using TLS and the distant server certificate as the root.
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(ta),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: defaultMinConnectTimeout,
		}))
	if err != nil {
		return nil, xerrors.Errorf("failed to create a dial connection: %v", err)
	}

	return conn, nil
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

// sender implements mino.Sender
type sender struct {
	sync.Mutex
	encoder      encoding.ProtoMarshaler
	address      address
	participants sync.Map
	name         string
	srvCert      *tls.Certificate
	traffic      *traffic
	routing      routing.Routing

	// this is the address of the orchestrator that own this client
	rootAddr address
}

// send implements mino.Sender.Send. This function sends the message
// asynchrously to all the addrs. The chan error is closed when all the send on
// each addrs are done. This function guarantees that the error chan is
// eventually closed. This function should not receive envelopes or OverlayMsgs
func (s *sender) Send(msg proto.Message, addrs ...mino.Address) <-chan error {
	return s.sendWithFrom(msg, s.address, addrs...)
}

// sendWithFrom is needed in the case we send a relayed message. In this case
// the "from" attribute sould be the original sender of the message that must be
// relayed, and not ourselve.
func (s *sender) sendWithFrom(msg proto.Message, from mino.Address, addrs ...mino.Address) <-chan error {

	errs := make(chan error, len(addrs))
	var wg sync.WaitGroup
	wg.Add(len(addrs))

	for _, addr := range addrs {
		go func(addr mino.Address) {
			defer wg.Done()
			err := s.sendSingle(msg, from, addr)
			if err != nil {
				err = xerrors.Errorf("sender '%s' failed to send to client '%s': %v", s.address.String(), addr, err)
				fabric.Logger.Err(err).Send()
				errs <- err
			}
		}(addr)
	}

	// waits for all the goroutine to end and close the errors chan
	go func() {
		wg.Wait()
		close(errs)
	}()

	return errs
}

// sendSingle sends a message to a single recipient. This function should be
// called asynchonously for each addrs set in mino.Sender.Send
func (s *sender) sendSingle(msg proto.Message, from, to mino.Address) error {
	s.Lock()
	defer s.Unlock()

	msgAny, err := s.encoder.MarshalAny(msg)
	if err != nil {
		return xerrors.Errorf("couldn't marshal message: %v", err)
	}

	routingTo, err := s.routing.GetRoute(s.address, to)
	if err != nil {
		// If we can't get a route we send the message to the orchestrator. In a
		// tree based communication this means sending the message to our
		// parent.
		routingTo = s.address
	}
	// fmt.Println("routing from", s.address, "to", to, "=", routingTo, "participants", s.participants)

	logTo := routingTo
	// This is the case we are responding to our stream, which replies to the
	// orchestrator that created the stream.
	if routingTo == s.address {
		logTo = s.rootAddr
	}

	playerItf, participantFound := s.participants.Load(routingTo.String())
	if !participantFound {
		return xerrors.Errorf("failed to send a message to my child '%s', "+
			"participant not found in '%v'", routingTo, s.participants)
	}

	player, ok := playerItf.(overlayStream)
	if !ok {
		fabric.Logger.Fatal().Msg("not ok")
	}

	envelope := &Envelope{
		From:         from.String(),
		PhysicalFrom: s.address.String(),
		To:           []string{to.String()},
		Message:      msgAny,
	}

	s.traffic.logSend(s.address, logTo, envelope, s.name)
	err = player.Send(envelope)
	if err == io.EOF {
		return nil
	} else if err != nil {
		return xerrors.Errorf("failed to call the send on client stream: %v", err)
	}

	return nil
}

// receiver implements mino.receiver
type receiver struct {
	encoder encoding.ProtoMarshaler
	errs    chan error
	in      chan *Envelope
	name    string
	traffic *traffic
}

// Recv implements mino.receiver
func (r receiver) Recv(ctx context.Context) (mino.Address, proto.Message, error) {
	// TODO: close the channel
	var enveloppe *Envelope
	var err error
	var ok bool

	select {
	case enveloppe, ok = <-r.in:
		if !ok {
			return nil, nil, errors.New("time to end")
		}
	case err = <-r.errs:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	if err != nil {
		return nil, nil, xerrors.Errorf("got an error from the error chan: %v", err)
	}

	// we check it to prevent a panic on msg.Message
	if enveloppe == nil {
		return nil, nil, xerrors.New("message is nil")
	}

	msg, err := r.encoder.UnmarshalDynamicAny(enveloppe.Message)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to unmarshal enveloppe msg: %v", err)
	}

	return address{id: enveloppe.From}, msg, nil
}

// This interface is used to have a common object between Overlay_StreamServer
// and Overlay_StreamClient. We need it because the orchestrator is setting
// Overlay_StreamClient, while the RPC (in overlay.go) is setting an
// Overlay_StreamServer
type overlayStream interface {
	Send(*Envelope) error
	Recv() (*Envelope, error)
}

type safeResponse struct {
	msg *Envelope
	err error
}

// safeOverlayStream is a wrapper around overlayStream that guarantees the Send
// and Recv are executed in a single go routine each, as not doing so could be a
// problem for grpc.
type safeOverlayStream struct {
	sendMux       sync.Mutex
	sendChan      chan *Envelope
	sendErrorChan chan error
	recvChan      chan *safeResponse
}

func newSafeOverlayStream(o overlayStream) *safeOverlayStream {
	safe := &safeOverlayStream{
		sendChan:      make(chan *Envelope),
		sendErrorChan: make(chan error),
		recvChan:      make(chan *safeResponse),
	}

	go func() {
		for {
			msg, err := o.Recv()
			safe.recvChan <- &safeResponse{msg, err}
		}
	}()

	go func() {
		for {
			msg := <-safe.sendChan
			safe.sendErrorChan <- o.Send(msg)
		}
	}()

	return safe
}

func (s *safeOverlayStream) Send(msg *Envelope) error {
	s.sendMux.Lock()
	s.sendChan <- msg
	err := <-s.sendErrorChan
	s.sendMux.Unlock()

	return err
}

func (s *safeOverlayStream) Recv() (*Envelope, error) {
	resp := <-s.recvChan
	return resp.msg, resp.err
}
