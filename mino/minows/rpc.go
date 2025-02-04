package minows

import (
	"context"
	"encoding/gob"
	"errors"
	"sync"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const pathCall = "/call"
const pathStream = "/stream"

// packet encapsulates a message sent over the network streams.
type packet struct {
	Source      []byte
	Payload     []byte
	ForwardDest *[]byte
}

type envelope struct {
	author mino.Address
	msg    serde.Message
	err    error
}

// RPC
// - implements mino.RPC
type rpc struct {
	logger zerolog.Logger

	uri     string
	handler mino.Handler
	mino    *Minows
	factory serde.Factory
	context serde.Context
}

func (r rpc) Call(
	ctx context.Context, req serde.Message,
	players mino.Players,
) (<-chan mino.Response, error) {
	if players == nil || players.Len() == 0 {
		resp := make(chan mino.Response)
		close(resp)
		return resp, nil
	}

	addrs, err := toAddresses(players)
	if err != nil {
		return nil, err
	}

	r.addPeers(addrs)

	result := make(chan envelope, len(addrs))
	for _, addr := range addrs {
		if r.mino.myAddr.Equal(addr) {
			request := mino.Request{Address: r.mino.myAddr, Message: req}
			reply, err := r.handler.Process(request)
			result <- envelope{r.mino.myAddr, reply, err}
		} else {
			go func(addr address) {
				reply, err := r.unicast(ctx, addr, req)
				result <- envelope{addr, reply, err}
			}(addr)
		}
	}

	responses := make(chan mino.Response, len(addrs))
	go func() {
		defer close(responses)
		for i := 0; i < len(addrs); i++ {

			select {
			case <-ctx.Done():
				return
			case env := <-result:
				if env.err != nil {
					responses <- mino.NewResponseWithError(env.author, env.err)
				} else {
					responses <- mino.NewResponse(env.author, env.msg)
				}
			}
		}
	}()
	return responses, nil
}

func (r rpc) Stream(ctx context.Context, players mino.Players) (mino.Sender, mino.Receiver, error) {
	if players == nil || players.Len() == 0 {
		return nil, nil, xerrors.New("no players")
	}

	initiator, err := libp2p.New(libp2p.NoListenAddrs)
	if err != nil {
		return nil, nil, xerrors.Errorf("could not start host: %v", err)
	}

	go func() {
		<-ctx.Done()
		err := initiator.Close()
		if err != nil {
			r.logger.Error().Err(err).Msg("could not close host")
		}
	}()

	errs := make(chan error, players.Len())
	streams := make(chan network.Stream, players.Len())
	var wg sync.WaitGroup
	wg.Add(players.Len())
	for iter := players.AddressIterator(); iter.HasNext(); {
		player := iter.GetNext()
		addr, ok := player.(address)
		if !ok {
			return nil, nil, xerrors.Errorf("%v: %T",
				ErrWrongAddressType, player)
		}

		go func(addr address) {
			defer wg.Done()
			initiator.Peerstore().AddAddr(addr.identity, addr.location,
				peerstore.PermanentAddrTTL)

			stream, err := initiator.NewStream(ctx, addr.identity,
				protocol.ID(r.uri+pathStream))
			if err != nil {
				errs <- xerrors.Errorf("could not open stream: %v", err)
				return
			}
			streams <- stream

			go func() {
				<-ctx.Done()
				err = stream.Reset()
				if err != nil {
					r.logger.Error().Err(err).Msg("could not reset stream")
				}
			}()
		}(addr)
	}

	wg.Wait()
	close(errs)
	close(streams)
	for err := range errs {
		return nil, nil, err
	}

	opened := make([]network.Stream, 0, players.Len())
	for stream := range streams {
		opened = append(opened, stream)
	}
	o, err := r.createOrchestrator(ctx, initiator, opened)
	if err != nil {
		return nil, nil, xerrors.Errorf("could not create orchestrator: %v", err)
	}
	return o, o, nil
}

func (r rpc) handleCall(stream network.Stream) {
	dec := gob.NewDecoder(stream)
	from, req, err := r.receive(dec)
	if err != nil {
		r.logger.Error().Err(err).Msg("could not receive")
	}

	id := stream.Conn().RemotePeer()
	author := address{location: from, identity: id}
	reply, err := r.handler.Process(mino.Request{Address: author, Message: req})
	if err != nil {
		r.logger.Error().Err(err).Msg("could not process")
	}

	enc := gob.NewEncoder(stream)
	err = r.send(enc, reply)
	if err != nil {
		r.logger.Error().Err(err).Msg("could not reply")
	}
}

func (r rpc) handleStream(stream network.Stream) {
	p := r.createParticipant(stream)

	go func() {
		err := r.handler.Stream(p, p)
		if err != nil {
			r.logger.Error().Err(err).Msg("could not handle stream")
		}
	}()
}

func toAddresses(players mino.Players) ([]address, error) {
	addrs := make([]address, 0, players.Len())
	iter := players.AddressIterator()
	for iter.HasNext() {
		player := iter.GetNext()
		addr, ok := player.(address)
		if !ok {
			return nil, xerrors.Errorf("wrong address type: %T", player)
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

func (r rpc) addPeers(addrs []address) {
	for _, addr := range addrs {
		r.mino.host.Peerstore().AddAddr(addr.identity, addr.location,
			peerstore.PermanentAddrTTL)
	}
}

func (r rpc) unicast(ctx context.Context, dest address, req serde.Message) (
	serde.Message, error,
) {
	stream, err := r.openStream(ctx, dest, pathCall)
	if err != nil {
		return nil, xerrors.Errorf("could not open stream: %v", err)
	}

	dec := gob.NewEncoder(stream)
	err = r.send(dec, req)
	if err != nil {
		return nil, xerrors.Errorf("could not send request: %v", err)
	}

	enc := gob.NewDecoder(stream)
	_, reply, err := r.receive(enc)
	if err != nil {
		return nil, xerrors.Errorf("could not receive reply: %v", err)
	}
	return reply, nil
}

func (r rpc) openStream(
	ctx context.Context, dest address,
	path string,
) (network.Stream, error) {
	pid := protocol.ID(r.uri + path)
	stream, err := r.mino.host.NewStream(ctx, dest.identity, pid)
	if err != nil {
		return nil, xerrors.Errorf("could not open stream: %v", err)
	}

	go func() {
		<-ctx.Done()
		err := stream.Reset()
		if err != nil {
			r.logger.Error().Err(err).Msg("could not reset stream")
		}
	}()

	return stream, nil
}

func (r rpc) send(enc *gob.Encoder, msg serde.Message) error {
	from := r.mino.myAddr.location.Bytes()

	var payload []byte
	if msg != nil {
		bytes, err := msg.Serialize(r.context)
		if err != nil {
			return xerrors.Errorf("could not serialize message: %v", err)
		}
		payload = bytes
	}

	err := enc.Encode(&packet{Source: from, Payload: payload})
	if errors.Is(err, network.ErrReset) {
		return err
	}
	if err != nil {
		return xerrors.Errorf("could not encode packet: %v", err)
	}
	return nil
}

func (r rpc) receive(dec *gob.Decoder) (ma.Multiaddr, serde.Message, error) {
	var pkt packet
	err := dec.Decode(&pkt)
	if errors.Is(err, network.ErrReset) {
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, xerrors.Errorf("could not decode packet: %v", err)
	}

	from, err := ma.NewMultiaddrBytes(pkt.Source)
	if err != nil {
		return nil, nil, xerrors.Errorf("could not unmarshal address: %v",
			pkt.Source)
	}

	if pkt.Payload == nil {
		return from, nil, nil
	}
	msg, err := r.factory.Deserialize(r.context, pkt.Payload)
	if err != nil {
		return from, nil, xerrors.Errorf(
			"could not deserialize message: %v",
			err)
	}
	return from, msg, nil
}

func (r rpc) createOrchestrator(
	ctx context.Context,
	initiator host.Host, streams []network.Stream,
) (*messageHandler, error) {
	participant := r.mino.GetAddress().(address).location
	myAddr, err := newOrchestratorAddr(participant, initiator.ID())
	if err != nil {
		return nil, xerrors.Errorf("could not create address: %v", err)
	}

	encoders := make(map[peer.ID]*gob.Encoder)
	for _, stream := range streams {
		encoders[stream.Conn().RemotePeer()] = gob.NewEncoder(stream)
	}

	m := &messageHandler{
		logger: r.logger.With().Stringer("mino", myAddr).
			Stringer("orchestrator", xid.New()).Logger(),
		myAddr:  myAddr,
		rpc:     r,
		streams: streams,
		outs:    encoders,
		in:      make(chan packet, MaxUnreadAllowed),
	}

	m.loop(ctx)
	return m, nil
}

func (r rpc) createParticipant(stream network.Stream) messageHandler {
	out := gob.NewEncoder(stream)

	ctx, cancel := context.WithCancel(context.Background())
	m := messageHandler{
		logger:  r.logger.With().Stringer("participant", xid.New()).Logger(),
		myAddr:  r.mino.myAddr,
		rpc:     r,
		cancel:  cancel,
		streams: []network.Stream{stream},
		outs:    map[peer.ID]*gob.Encoder{stream.Conn().RemotePeer(): out},
		in:      make(chan packet),
	}

	m.loop(ctx)

	return m
}
