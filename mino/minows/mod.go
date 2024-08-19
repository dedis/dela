package minows

import (
	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/serde/json"
	"regexp"
	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

var pattern = regexp.MustCompile("^[a-zA-Z0-9]+$")

// Minows
// - implements mino.Mino
type minows struct {
	logger zerolog.Logger

	myAddr   address
	host     host.Host
	segments []string
	rpcs     map[string]any
	factory  addressFactory
}

// NewMinows creates a new Minows instance that starts listening.
// listen: listening address in multiaddress format,
// e.g. /ip4/127.0.0.1/tcp/80/ws
// public: public dial-able address in multiaddress format,
// e.g. /dns4/p2p-1.c4dt.dela.org/tcp/443/wss
// `public` can be nil and will be determined
// by the listening address and the port the host has bound to.
// key: private key representing this mino instance's identity
func NewMinows(listen, public ma.Multiaddr, key crypto.PrivKey) (mino.Mino,
	error) {
	h, err := libp2p.New(libp2p.ListenAddrs(listen), libp2p.Identity(key))
	if err != nil {
		return nil, xerrors.Errorf("could not start host: %v", err)
	}

	if public == nil {
		public = h.Addrs()[0]
	}
	myAddr, err := newAddress(public, h.ID())
	if err != nil {
		return nil, xerrors.Errorf("could not create address: %v", err)
	}

	return &minows{
		logger:   dela.Logger.With().Str("mino", myAddr.String()).Logger(),
		myAddr:   myAddr,
		segments: nil,
		host:     h,
		rpcs:     make(map[string]any),
		factory:  addressFactory{},
	}, nil
}

func (m *minows) GetAddressFactory() mino.AddressFactory {
	return m.factory
}

func (m *minows) GetAddress() mino.Address {
	return m.myAddr
}

func (m *minows) WithSegment(segment string) mino.Mino {
	if segment == "" {
		return m
	}

	return &minows{
		logger:   m.logger,
		myAddr:   m.myAddr,
		segments: append(m.segments, segment),
		host:     m.host,
		rpcs:     make(map[string]any),
		factory:  addressFactory{},
	}
}

func (m *minows) CreateRPC(name string, h mino.Handler, f serde.Factory) (mino.RPC, error) {
	if len(m.rpcs) == 0 {
		for _, seg := range m.segments {
			if !pattern.MatchString(seg) {
				return nil, xerrors.Errorf("invalid segment: %s", seg)
			}
		}
	}

	if !pattern.MatchString(name) {
		return nil, xerrors.Errorf("invalid name: %s", name)
	}
	_, found := m.rpcs[name]
	if found {
		return nil, xerrors.Errorf("already exists rpc: %s", name)
	}

	uri := strings.Join(append(m.segments, name), "/")

	r := &rpc{
		logger:  m.logger.With().Str("rpc", uri).Logger(),
		uri:     uri,
		handler: h,
		mino:    m,
		factory: f,
		context: json.NewContext(),
	}

	m.host.SetStreamHandler(protocol.ID(uri+pathCall), r.handleCall)
	m.host.SetStreamHandler(protocol.ID(uri+pathStream), r.handleStream)
	m.rpcs[name] = nil

	return r, nil
}

func (m *minows) stop() error {
	return m.host.Close()
}
