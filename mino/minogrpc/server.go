package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	fmt "fmt"
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
	// in a tree based communication, this parameter (H) defines the height of
	// the tree. Based on this parameter and the total number of nodes N we can
	// compute the number of direct connection D for each node with D = N^(1/H)
	treeHeight = 6
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
	routingFactory RoutingFactory

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
	routingFactory RoutingFactory
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

	sendMsg := &OverlayMsg{
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

	addrs := make([]mino.Address, 0, players.Len())
	for players.AddressIterator().HasNext() {
		addrs = append(addrs, players.AddressIterator().GetNext())
	}
	addrsStr := make([]string, len(addrs))
	for i, addr := range addrs {
		addrsStr[i] = addr.String()
	}

	// warning: this call will shuffle addrs

	routing, err := rpc.routingFactory.FromAddrs(addrs, map[string]interface{}{
		TreeRoutingOpts.Addr:       address{orchestratorAddr},
		TreeRoutingOpts.TreeHeight: treeHeight,
	})
	if err != nil {
		// TODO better handle this error
		fabric.Logger.Fatal().Msgf("failed to create routing: %v", err)
	}

	fmt.Println("tree topology:", routing.(*TreeRouting).root)

	routingProto := &RoutingMsg{Type: "tree", Addrs: addrsStr}

	anyRoutintg, err := rpc.encoder.MarshalAny(routingProto)
	if err != nil {
		fabric.Logger.Fatal().Msgf("failed to encode routing info: %v", err)
	}

	// if every player produces an error the buffer should be large enought so
	// that we are never blocked in the for loop and we can termninate this
	// function.
	errs := make(chan error, players.Len())

	orchSender := &sender{
		encoder: rpc.encoder,
		address: address{orchestratorAddr},
		// Participant should contain the stream connection of every child of
		// this node
		participants: make(map[string]overlayStream),
		name:         "orchestrator",
		srvCert:      rpc.srv.cert,
		traffic:      rpc.srv.traffic,
		routing:      routing,
	}

	orchRecv := receiver{
		encoder: rpc.encoder,
		errs:    errs,
		// it is okay to have a blocking chan here because every use of it is in
		// a goroutine, where we don't mind if it blocks.
		in:      make(chan *OverlayMsg),
		name:    "orchestrator",
		traffic: rpc.srv.traffic,
	}

	// Creating a stream for each addr in our tree routing
	for _, addr := range routing.GetDirectLinks() {

		peer, ok := rpc.srv.neighbours[addr.String()]
		if !ok {
			err := xerrors.Errorf("addr '%s' not is our list of neighbours", addr)
			fabric.Logger.Err(err).Send()
			errs <- err
			continue
		}

		clientConn, err := getConnection(addr.String(), peer, *rpc.srv.cert)
		if err != nil {
			// TODO: try another path (maybe use another node to relay that
			// message)
			err = xerrors.Errorf("failed to get client conn for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			errs <- err
			continue
		}
		cl := NewOverlayClient(clientConn)

		header := metadata.New(map[string]string{headerURIKey: rpc.uri})
		newCtx := metadata.NewOutgoingContext(ctx, header)

		stream, err := cl.Stream(newCtx)
		if err != nil {
			err = xerrors.Errorf("failed to get stream for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			errs <- err
			continue
		}

		orchSender.participants[addr.String()] = stream

		// Sending the routing info as first messages to our childs
		safeStream.Send(&OverlayMsg{Message: anyRoutintg})

		// Listen on the clients streams and notify the orchestrator or relay
		// messages
		go func(addr mino.Address) {
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
		}(addr)
	}

	return orchSender, orchRecv
}

// listenStream reads the client RPC stream and handle the received messages
// accordingly: It formwards messages or notify the orchestrator.
func listenStream(stream overlayStream, orchRecv *receiver,
	orchSender *sender, addr mino.Address) error {

	// This msg.Message should always be an enveloppe
	msg, err := stream.Recv()
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

	envelope := &Envelope{}
	err = orchRecv.encoder.UnmarshalAny(msg.Message, envelope)
	if err != nil {
		return xerrors.Errorf("couldn't unmarshal envelope: %v", err)
	}

	message, err := orchRecv.encoder.UnmarshalDynamicAny(envelope.Message)
	if err != nil {
		return xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	for _, toSend := range envelope.To {
		// if we receive a message to the orchestrator then we notify the
		// orchestrator receiver by filling orchRecv.in. If this is not the case
		// we relay the message.
		if toSend == orchSender.address.String() {
			orchRecv.in <- msg
		} else {
			orchRecv.traffic.logRcvRelay(address{envelope.From}, envelope,
				orchRecv.name)
			fabric.Logger.Trace().Msgf("(orchestrator) relaying message from "+
				"'%s' to '%s'", envelope.From, toSend)

			errChan := orchSender.sendWithFrom(message,
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
func NewServer(addr mino.Address, rf RoutingFactory) (*Server, error) {
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
	participants map[string]overlayStream
	name         string
	srvCert      *tls.Certificate
	traffic      *traffic
	routing      Routing
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

	routingTo, err := s.routing.GetRoute(to)
	if err != nil {
		// If we can't get a route we send the message to the orchestrator. In a
		// tree based communication this means sending the message to our
		// parent.
		routingTo = s.address
	}

	player, participantFound := s.participants[routingTo.String()]
	if !participantFound {
		return xerrors.Errorf("failed to send a message to my child '%s', "+
			"participant not found in '%v'", routingTo, s.participants)
	}

	envelope := &Envelope{
		From:    from.String(),
		To:      []string{to.String()},
		Message: msgAny,
	}
	envelopeAny, err := s.encoder.MarshalAny(envelope)
	if err != nil {
		return xerrors.Errorf("couldn't marshal envelope: %v", err)
	}

	sendMsg := &OverlayMsg{
		Message: envelopeAny,
	}

	s.traffic.logSend(to, sendMsg, s.name)
	err = player.Send(sendMsg)
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
	in      chan *OverlayMsg
	name    string
	traffic *traffic
}

// Recv implements mino.receiver
func (r receiver) Recv(ctx context.Context) (mino.Address, proto.Message, error) {
	// TODO: close the channel
	var msg *OverlayMsg
	var err error
	var ok bool

	select {
	case msg, ok = <-r.in:
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
	if msg == nil {
		return nil, nil, xerrors.New("message is nil")
	}

	enveloppe := &Envelope{}
	err = r.encoder.UnmarshalAny(msg.Message, enveloppe)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't unmarshal envelope: %v", err)
	}

	message, err := r.encoder.UnmarshalDynamicAny(enveloppe.Message)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	r.traffic.logRcv(address{enveloppe.From}, message, r.name)

	return address{id: enveloppe.From}, message, nil
}

// This interface is used to have a common object between Overlay_StreamServer
// and Overlay_StreamClient. We need it because the orchastrator is setting
// Overlay_StreamClient, while the RPC (in overlay.go) is setting an
// Overlay_StreamServer
type overlayStream interface {
	Send(*OverlayMsg) error
	Recv() (*OverlayMsg, error)
}

type safeOverlayStream struct {
	sendLock sync.Mutex
	recvLock sync.Mutex
	os       overlayStream
}

func newSafeOverlayStream(o overlayStream) *safeOverlayStream {
	return &safeOverlayStream{
		os: o,
	}
}

func (os *safeOverlayStream) Send(msg *OverlayMsg) error {
	// os.sendLock.Lock()
	// defer os.sendLock.Unlock()
	return os.os.Send(msg)
}

func (os *safeOverlayStream) Recv() (*OverlayMsg, error) {
	// os.recvLock.Lock()
	// defer os.recvLock.Unlock()
	return os.os.Recv()
}
