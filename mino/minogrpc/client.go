package minogrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"math/big"
	"net"
	"net/url"
	"sync"
	"time"
)

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
		panic(
			xerrors.Errorf(
				"certificate of the overlay is inaccessible: %v", err,
			),
		)
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

	buf, err := x509.CreateCertificate(
		rand.Reader, tmpl, tmpl, o.public, o.secret,
	)
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
