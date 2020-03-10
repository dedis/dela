package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	// defaultMinConnectTimeout is the minimum amount of time we are willing to
	// wait for a grpc connection to complete
	defaultMinConnectTimeout = 7 * time.Second
	// defaultContextTimeout is the amount of time we are willing to wait for a
	// remote procedure call to finish. This value should always be higher than
	// defaultMinConnectTimeout in order to capture http server errors.
	defaultContextTimeout = 10 * time.Second
)

// Server represents the entity that accepts incoming requests and invoke the
// corresponding RPCs.
type Server struct {
	grpcSrv *grpc.Server

	cert      *tls.Certificate
	addr      *mino.Address
	listener  net.Listener
	httpSrv   *http.Server
	StartChan chan struct{}

	// neighbours contains the certificate and details about known peers.
	neighbours map[string]Peer

	handlers map[string]mino.Handler

	localStreamClients map[string]Overlay_StreamClient
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

// RPC represents an RPC that has been registered by a client. This struct
// implements the mino.RPC interface, which allows clients to call an RPC that
// will execute the provided handler.
type RPC struct {
	handler mino.Handler
	srv     Server
	uri     string
}

// Call implements the Mino.RPC interface. It calls the RPC on each provided
// address.
func (rpc RPC) Call(req proto.Message,
	addrs ...*mino.Address) (<-chan proto.Message, <-chan error) {

	out := make(chan proto.Message, len(addrs))
	errs := make(chan error, len(addrs))

	m, err := ptypes.MarshalAny(req)
	if err != nil {
		errs <- xerrors.Errorf("failed to marshal msg to any: %v", err)
		return out, errs
	}

	sendMsg := &OverlayMsg{
		Message: m,
	}

	go func() {
		for _, addr := range addrs {
			clientConn, err := rpc.srv.getConnection(addr.GetId())
			if err != nil {
				errs <- xerrors.Errorf("failed to get client conn: %v", err)
				continue
			}
			cl := NewOverlayClient(clientConn)

			ctx, ctxCancelFunc := context.WithTimeout(context.Background(),
				defaultContextTimeout)
			defer ctxCancelFunc()

			header := metadata.New(map[string]string{"apiuri": rpc.uri})
			ctx = metadata.NewOutgoingContext(ctx, header)

			callResp, err := cl.Call(ctx, sendMsg)
			if err != nil {
				errs <- xerrors.Errorf("failed to call client: %v", err)
				continue
			}

			var resp ptypes.DynamicAny
			err = ptypes.UnmarshalAny(callResp.Message, &resp)

			out <- resp.Message
		}

		close(out)
	}()

	return out, errs
}

// CreateServer sets up a new server
func CreateServer(addr *mino.Address) (*Server, error) {
	cert, err := makeCertificate()
	if err != nil {
		return nil, xerrors.Errorf("failed to make certificate: %v", err)
	}

	srv := grpc.NewServer()

	server := &Server{
		grpcSrv:    srv,
		cert:       cert,
		addr:       addr,
		listener:   nil,
		StartChan:  make(chan struct{}),
		neighbours: make(map[string]Peer),
		handlers:   make(map[string]mino.Handler),
	}

	RegisterOverlayServer(srv, &overlayService{
		handlers: server.handlers,
	})

	return server, nil
}

// StartServer makes the server start listening and wait until its started
func (srv *Server) StartServer() error {

	go func() {
		err := srv.Serve()
		// TODO: better handle this error
		if err != nil {
			fabric.Logger.Fatal().Msg("failed to start server " + err.Error())
		}
	}()

	<-srv.StartChan

	return nil
}

// Serve starts the HTTP server that forwards gRPC calls to the gRPC server
func (srv *Server) Serve() error {
	lis, err := net.Listen("tcp4", srv.addr.Id)
	if err != nil {
		return xerrors.Errorf("failed to listen: %v", err)
	}

	srv.listener = lis

	close(srv.StartChan)

	wrapped := grpcweb.WrapServer(srv.grpcSrv)

	srv.httpSrv = &http.Server{
		TLSConfig: &tls.Config{
			// TODO: a LE certificate or similar must be used alongside the
			// actual server certificate for the browser to accept the TLS
			// connection.
			Certificates: []tls.Certificate{*srv.cert},
			ClientAuth:   tls.RequestClientCert,
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") == "application/grpc" {
				srv.grpcSrv.ServeHTTP(w, r)
			} else {
				wrapped.ServeHTTP(w, r)
			}
		}),
	}

	// This call always returns an error
	err = srv.httpSrv.ServeTLS(lis, "", "")
	if err != http.ErrServerClosed {
		return xerrors.Errorf("failed to serve: %v", err)
	}

	return nil
}

// getConnection creates a gRPC connection from the server to the client
func (srv *Server) getConnection(addr string) (*grpc.ClientConn, error) {
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

func makeCertificate() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("Couldn't generate the private key: %+v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 180),

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

// Stream ...
func (rpc RPC) Stream(ctx context.Context,
	addrs ...*mino.Address) (mino.Sender, mino.Receiver) {

	errs := make(chan error, 1)

	orchSender := sender{
		addr:         &mino.Address{},
		participants: make([]player, len(addrs)),
	}

	// Creating the local stream
	// clientConn, err := rpc.srv.getConnection(rpc.srv.addr.GetId())
	// if err != nil {
	// 	fabric.Logger.Fatal().Msgf("failed to get client conn: %v", err)
	// }
	// cl := NewOverlayClient(clientConn)

	// header := metadata.New(map[string]string{
	// 	"apiuri": rpc.uri,
	// 	"addr":   rpc.srv.addr.Id})
	// ctx = metadata.NewOutgoingContext(context.Background(), header)

	// stream, err := cl.Stream(ctx)
	// if err != nil {
	// 	fabric.Logger.Fatal().Msgf("failed to get stream from client: %v", err)
	// }

	orchRecv := receiver{
		errs: make(chan error, 1),
		in:   make(chan *OverlayMsg, 1),
	}

	// orchSender.participants[len(addrs)] = player{
	// 	addr:         &mino.Address{},
	// 	streamClient: rpc.srv.localStreamClients[rpc.uri],
	// }

	// Creating a stream for each provided addr
	fmt.Println("before the loop")
	for i, addr := range addrs {

		fmt.Println("doing an address")
		clientConn, err := rpc.srv.getConnection(addr.GetId())
		if err != nil {
			// TODO: essayer de passer par ailleurs
			errs <- xerrors.Errorf("failed to get client conn: %v", err)
			continue
		}
		cl := NewOverlayClient(clientConn)

		header := metadata.New(map[string]string{
			"apiuri": rpc.uri,
			"addr":   ""})
		ctx = metadata.NewOutgoingContext(ctx, header)

		stream, err := cl.Stream(ctx)
		if err != nil {
			fabric.Logger.Fatal().Msgf("failed to get stream from client: %v", err)
		}

		orchSender.participants[i] = player{
			addr:         addr,
			streamClient: stream,
		}

		go func() {
			msg, err := stream.Recv()
			if err != nil {
				fabric.Logger.Error().Msgf("failed to receive: %v", err)
			}

			orchRecv.in <- msg
		}()
	}

	return orchSender, orchRecv
}

// sender ...
type sender struct {
	addr         *mino.Address
	participants []player
}

type player struct {
	addr         *mino.Address
	streamClient overlayStream
}

// receiver ...
type receiver struct {
	errs chan error
	in   chan *OverlayMsg
}

// Recv ...
// TODO: check the error chan
func (r receiver) Recv(ctx context.Context) (*mino.Address, proto.Message, error) {
	fmt.Println("in the receiver.Recv()")

	// TODO: close the channel
	msg := <-r.in

	enveloppe := &mino.Envelope{}
	err := ptypes.UnmarshalAny(msg.Message, enveloppe)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to unmarshal enveloppe: %v", err)
	}

	var dynamicAny ptypes.DynamicAny
	err = ptypes.UnmarshalAny(enveloppe.Message, &dynamicAny)
	if err != nil {
		return nil, nil, encoding.NewAnyDecodingError(enveloppe.Message, err)
	}

	return enveloppe.From, dynamicAny.Message, nil
}

// This interface is used to have a common object between Overlay_StreamServer
// and Overlay_StreamClient
type overlayStream interface {
	Send(*OverlayMsg) error
	Recv() (*OverlayMsg, error)
}

func (s sender) getParticipant(addr *mino.Address) *player {
	for _, p := range s.participants {
		if p.addr.Id == addr.Id {
			return &p
		}
	}
	return nil
}

// send ...
func (s sender) Send(msg proto.Message, addrs ...*mino.Address) error {
	fmt.Println("in the sender.Send()", s.participants)

	for _, add := range addrs {

		player := s.getParticipant(add)
		if player == nil {
			fmt.Println("Participant not found: ", add)
			continue
		}

		msgAny, err := ptypes.MarshalAny(msg)
		if err != nil {
			return encoding.NewAnyEncodingError(msg, err)
		}

		envelope := &mino.Envelope{
			From:    s.addr,
			To:      []*mino.Address{add},
			Message: msgAny,
		}

		envelopeAny, err := ptypes.MarshalAny(envelope)
		if err != nil {
			return encoding.NewAnyEncodingError(msg, err)
		}

		sendMsg := &OverlayMsg{
			Message: envelopeAny,
		}

		fmt.Println("calling overlayServer.Send()")
		err = player.streamClient.Send(sendMsg)
		if err != nil {
			return xerrors.Errorf("failed to call the send on client stream: %v", err)
		}
	}

	return nil
}
