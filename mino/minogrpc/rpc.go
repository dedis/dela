package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	fmt "fmt"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
)

// GrpcRPC ...
type GrpcRPC struct {
	*grpc.Server

	cert      *tls.Certificate
	addr      string
	listener  net.Listener
	srv       *http.Server
	StartChan chan struct{}

	// neighbours contains the certificate and details about known peers.
	neighbours map[string]Peer
}

// Call ...
func (rpc GrpcRPC) Call(req proto.Message,
	addrs ...*mino.Address) (<-chan proto.Message, <-chan error) {

	out := make(chan proto.Message, len(addrs))
	errs := make(chan error, len(addrs))

	m, err := ptypes.MarshalAny(req)
	if err != nil {
		errs <- xerrors.Errorf("failed to marshal msg to any: %v", err)
		return out, errs
	}

	sendMsg := &CallMsg{
		Message: m,
	}

	go func() {
		for _, addr := range addrs {
			fmt.Println("Using addr ", addr)
			clientConn, err := rpc.getConnection(addr.GetId())
			if err != nil {
				errs <- xerrors.Errorf("failed to get client conn: %v", err)
				continue
			}
			cl := NewOverlayClient(clientConn)

			fmt.Println("Calling...")
			ctx, ctxCancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
			defer ctxCancelFunc()
			callResp, err := cl.Call(ctx, sendMsg)
			fmt.Println("Checking...")
			if err != nil {
				fmt.Println("Error:", err)
				errs <- xerrors.Errorf("failed to call client: %v", err)
				continue
			}
			fmt.Println("Got a response!")
			out <- callResp
		}

		close(out)
	}()

	return out, errs
}

func (rpc *GrpcRPC) getConnection(addr string) (*grpc.ClientConn, error) {
	neighbour, ok := rpc.neighbours[addr]
	if !ok {
		return nil, fmt.Errorf("couldn't find neighbour [%s]", addr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(neighbour.Certificate)

	ta := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*rpc.cert},
		RootCAs:      pool,
	})

	// Connecting using TLS and the distant server certificate as the root.
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(ta),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: 7 * time.Second,
		}))
	if err != nil {
		return nil, fmt.Errorf("couldn't dial: %v", err)
	}

	return conn, nil
}

// Peer is a public identity for a given node.
type Peer struct {
	Address     string
	Certificate *x509.Certificate
}

// Roster is a set of peers that will work together
// to execute protocols.
type Roster []Peer

func makeCertificate() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Couldn't generate the private key: %+v", err)
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
		return nil, fmt.Errorf("Couldn't create the certificate: %+v", err)
	}

	cert, err := x509.ParseCertificate(buf)
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse the certificate: %+v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{buf},
		PrivateKey:  priv,
		Leaf:        cert,
	}, nil
}

// gRPC service for the overlay.
type overlayService struct {
	overlayServer

	GrpcRPC *GrpcRPC
}

type overlayServer struct {
}

func (o overlayServer) Call(context.Context, *CallMsg) (*CallResp, error) {
	fmt.Println("Hi from the overlay Server")
	msg := mino.Address{
		Id: "blabla",
	}
	m, err := ptypes.MarshalAny(&msg)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal message: %v", err)
	}

	return &CallResp{Message: m}, nil
}

// Serve starts the overlay to listen on the address.
func (rpc *GrpcRPC) Serve() error {
	lis, err := net.Listen("tcp4", rpc.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	rpc.listener = lis

	close(rpc.StartChan)

	wrapped := grpcweb.WrapServer(rpc.Server)

	rpc.srv = &http.Server{
		TLSConfig: &tls.Config{
			// TODO: a LE certificate or similar must be used alongside the actual
			// server certificate for the browser to accept the TLS connection.
			Certificates: []tls.Certificate{*rpc.cert},
			ClientAuth:   tls.RequestClientCert,
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") == "application/grpc" {
				rpc.ServeHTTP(w, r)
			} else {
				wrapped.ServeHTTP(w, r)
			}
		}),
	}

	if err := rpc.srv.ServeTLS(lis, "", ""); err != nil {
		if err != http.ErrServerClosed {
			return fmt.Errorf("failed to serve: %v", err)
		}
	}

	return nil
}

// Stream ...
func (rpc GrpcRPC) Stream(ctx context.Context,
	addrs ...*mino.Address) (in mino.Sender, out mino.Receiver) {

	return nil, nil
}

// Sender ...
type Sender struct {
	Overlay OverlayServer
}

// Send sends msg to addrs, which should call the Receiver.Recv of each addrs.
func (s Sender) Send(msg proto.Message, addrs ...*mino.Address) error {

	return nil
}

// Receiver ...
type Receiver struct {
	overlay OverlayClient
}

// Recv ...
func (r Receiver) Recv(ctx context.Context) (*mino.Address, proto.Message, error) {
	return nil, nil, nil
}
