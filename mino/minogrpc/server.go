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
	"net/http"
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
	orchestratorID = "orchestrator"
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
	httpSrv   *http.Server
	StartChan chan struct{}

	// neighbours contains the certificate and details about known peers.
	neighbours map[string]Peer

	handlerRW *sync.RWMutex
	handlers  map[string]mino.Handler

	localStreamClients map[string]Overlay_StreamClient

	history history
}

type ctxURIKey string

const ctxKey = ctxURIKey("URI")

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
	handler mino.Handler
	srv     *Server
	uri     string
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
			clientConn, err := rpc.srv.getConnection(addrStr)
			if err != nil {
				errs <- xerrors.Errorf("failed to get client conn for '%s': %v",
					addrStr, err)
				continue
			}
			cl := NewOverlayClient(clientConn)

			header := metadata.New(map[string]string{headerURIKey: rpc.uri})
			ctx = metadata.NewOutgoingContext(ctx, header)

			callResp, err := cl.Call(ctx, sendMsg)
			if err != nil {
				errs <- xerrors.Errorf("failed to call client '%s': %v", addrStr, err)
				continue
			}

			var resp ptypes.DynamicAny
			err = ptypes.UnmarshalAny(callResp.Message, &resp)
			if err != nil {
				errs <- encoding.NewAnyDecodingError(resp, err)
				continue
			}

			out <- resp.Message
		}

		close(out)
	}()

	return out, errs
}

// Stream implements mino.RPC.
func (rpc RPC) Stream(ctx context.Context,
	players mino.Players) (in mino.Sender, out mino.Receiver) {

	// if every player produces an error the buffer should be large enought so
	// that we are never blocked in the for loop and we can termninate this
	// function.
	errs := make(chan error, players.Len())

	orchSender := &sender{
		address:      address{orchestratorID},
		participants: make(map[string]overlayStream),
		name:         "orchestrator",
		srv:          rpc.srv,
	}

	orchRecv := receiver{
		errs: errs,
		// it is okay to have a blocking chan here because every use of it is in
		// a goroutine, where we don't mind if it blocks.
		in:   make(chan *OverlayMsg),
		name: "orchestrator",
		srv:  rpc.srv,
	}

	// Creating a stream for each provided addr
	for i := 0; players.AddressIterator().HasNext(); i++ {
		addr := players.AddressIterator().GetNext()

		clientConn, err := rpc.srv.getConnection(addr.String())
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
		ctx = metadata.NewOutgoingContext(ctx, header)

		stream, err := cl.Stream(ctx)
		if err != nil {
			err = xerrors.Errorf("failed to get stream for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			errs <- err
			continue
		}
		orchSender.participants[addr.String()] = stream

		// Listen on the clients streams and notify the orchestrator or relay
		// messages
		go func() {
			for {
				err = listenClient(stream, orchRecv, *orchSender, addr)
				if err != nil {
					err = xerrors.Errorf("failed to listen stream: %v")
					fabric.Logger.Err(err).Send()
					errs <- err
					return
				}
			}
		}()
	}

	return orchSender, orchRecv
}

// listenClient reads the client RPC stream and handle the received messages
// accordingly: It formwards messages or notify the orchestrator.
func listenClient(stream Overlay_StreamClient, orchRecv receiver,
	orchSender sender, addr mino.Address) error {

	// This msg.Message should always be an enveloppe
	msg, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	status, ok := status.FromError(err)
	if ok && err != nil && status.Code() == codes.Canceled {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("client '%s' failed to "+
			"receive from RPC server: %v", addr.String(), err)
	}

	envelope := &Envelope{}
	err = ptypes.UnmarshalAny(msg.Message, envelope)
	if err != nil {
		return encoding.NewAnyDecodingError(envelope, err)
	}

	var dynamicAny ptypes.DynamicAny
	err = ptypes.UnmarshalAny(envelope.Message, &dynamicAny)
	if err != nil {
		return encoding.NewAnyDecodingError(envelope.Message, err)
	}

	for _, toSend := range envelope.To {
		// if we receive a message to ourself or the orchestrator, then we
		// notify the orchstrator receiver by filling orchRecv.in. If this is
		// not the case we then relay the message.
		if toSend == addr.String() || toSend == orchestratorID {
			orchRecv.in <- msg
		} else {
			orchRecv.srv.logRcvRelay(address{envelope.From}, envelope, orchRecv.name)
			fabric.Logger.Trace().Msgf("(orchestrator) relaying message from "+
				"'%s' to '%s'", envelope.From, toSend)

			errChan := orchSender.Send(dynamicAny.Message, address{toSend})
			err, more := <-errChan
			if more {
				return xerrors.Errorf("failed to send relay message: %v", err)
			}
		}
	}
	return nil
}

// CreateServer sets up a new server
func CreateServer(addr mino.Address) (*Server, error) {
	if addr.String() == "" {
		return nil, xerrors.New("addr.String() should not give an empty string")
	}

	cert, err := makeCertificate()
	if err != nil {
		return nil, xerrors.Errorf("failed to make certificate: %v", err)
	}

	srv := grpc.NewServer(grpc.Creds(credentials.NewServerTLSFromCert(cert)))
	var m *sync.RWMutex
	server := &Server{
		grpcSrv:    srv,
		cert:       cert,
		addr:       addr,
		listener:   nil,
		StartChan:  make(chan struct{}),
		neighbours: make(map[string]Peer),
		handlers:   make(map[string]mino.Handler),
		handlerRW:  m,
		history:    history{items: make([]item, 0)},
	}

	RegisterOverlayServer(srv, &overlayService{
		handlers:    server.handlers,
		handlerRcvr: make(map[string]receiver),
		addr:        address{addr.String()},
		srv:         server,
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

// getConnection creates a gRPC connection from the server to the client
func (srv *Server) getConnection(addr string) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, xerrors.New("empty address is not allowed")
	}

	neighbour, ok := srv.neighbours[addr]
	if !ok {
		return nil, xerrors.Errorf("couldn't find neighbour [%s]", addr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(neighbour.Certificate)

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*srv.cert},
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

func (srv *Server) logSend(to mino.Address, msg proto.Message, context string) {
	srv.history.addItem("send", to, msg, context)
}

func (srv *Server) logRcv(from mino.Address, msg proto.Message, context string) {
	srv.history.addItem("received", from, msg, context)
}

func (srv *Server) logRcvRelay(from mino.Address, msg proto.Message, context string) {
	srv.history.addItem("received to relay", from, msg, context)
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
	address      address
	participants map[string]overlayStream
	name         string
	srv          *Server
}

type player struct {
	address      address
	streamClient overlayStream
}

// send implements mino.Sender.Send. This function sends the message
// asynchrously to all the addrs. The chan error is closed when all the send on
// each addrs are done. This function guarantees that the error chan is
// eventually closed. This function should not receive envelopes or OverlayMsgs
func (s *sender) Send(msg proto.Message, addrs ...mino.Address) <-chan error {

	errs := make(chan error, len(addrs))
	var wg sync.WaitGroup
	wg.Add(len(addrs))

	for _, addr := range addrs {
		go func(addr mino.Address) {
			defer wg.Done()
			err := sendSingle(s, msg, addr)
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
func sendSingle(s *sender, msg proto.Message, addr mino.Address) error {
	msgAny, err := ptypes.MarshalAny(msg)
	if err != nil {
		return encoding.NewAnyEncodingError(msg, err)
	}

	player, participantFound := s.participants[addr.String()]
	if !participantFound {
		// The first option is to create a new connection if we have this
		// address in our list of neighbors. Otherwise we wrap the message in an
		// enveloppe and send it back to the orchestrator that will relay that
		// message.
		_, neighbourFound := s.srv.neighbours[addr.String()]

		if neighbourFound {
			clientConn, err := s.srv.getConnection(addr.String())
			if err != nil {
				return xerrors.Errorf("failed to create new client in send: %v", err)
			}
			cl := NewOverlayClient(clientConn)

			header := metadata.New(map[string]string{headerURIKey: "blabla"})
			ctx := metadata.NewOutgoingContext(context.Background(), header)

			player, err = cl.Stream(ctx)
			if err != nil {
				return xerrors.Errorf("failed to create new stream from new client: %v", err)
			}

			// we add this new participant to our list in case we have to send
			// again a message to him
			s.participants[addr.String()] = player

		} else {
			// If we don't have it as neighbour our last option is to ask the
			// orchestrator to relay the message for us. In this case we send
			// back a message using our client, which will hit the orchestrator
			// that will see that this message should be relayed thanks to the
			// enveloppe.
			player, participantFound = s.participants[s.address.String()]
			if !participantFound {
				return xerrors.Errorf("failed to send back a message that should "+
					"be relayed to '%s'. My client '%s' was not found in the list "+
					"of participant: '%v'", addr, s.address, s.participants)
			}

			fabric.Logger.Trace().Msgf("I don't know client '%s', so I'm sending "+
				"back a message that must be relayed. From '%s', To '%s'",
				addr.String(), s.address.String(), addr.String())

		}
	}

	envelope := &Envelope{
		From:    s.address.String(),
		To:      []string{addr.String()},
		Message: msgAny,
	}
	envelopeAny, err := ptypes.MarshalAny(envelope)
	if err != nil {
		return encoding.NewAnyEncodingError(msg, err)
	}

	sendMsg := &OverlayMsg{
		Message: envelopeAny,
	}

	s.srv.logSend(addr, sendMsg, s.name)
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
	errs chan error
	in   chan *OverlayMsg
	name string
	stop chan interface{}
	srv  *Server
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
	err = ptypes.UnmarshalAny(msg.Message, enveloppe)
	if err != nil {
		return nil, nil, encoding.NewAnyDecodingError(enveloppe, err)
	}

	var dynamicAny ptypes.DynamicAny
	err = ptypes.UnmarshalAny(enveloppe.Message, &dynamicAny)
	if err != nil {
		return nil, nil, encoding.NewAnyDecodingError(enveloppe.Message, err)
	}

	r.srv.logRcv(address{enveloppe.From}, dynamicAny.Message, r.name)

	return address{id: enveloppe.From}, dynamicAny.Message, nil
}

// This interface is used to have a common object between Overlay_StreamServer
// and Overlay_StreamClient. We need it because the orchastrator is setting
// Overlay_StreamClient, while the RPC (in overlay.go) is setting an
// Overlay_StreamServer
type overlayStream interface {
	Send(*OverlayMsg) error
	Recv() (*OverlayMsg, error)
}

// simpleNode implements mino.Node
type simpleNode struct {
	addr address
}

// getAddress implements mino.Node
func (o simpleNode) GetAddress() mino.Address {
	return o.addr
}
