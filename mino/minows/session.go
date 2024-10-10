package minows

import (
	"context"
	"encoding/gob"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/rs/zerolog"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"io"
	"strings"
	"sync"
	"time"
)

var ErrWrongAddressType = xerrors.New("wrong address type")
var ErrNotPlayer = xerrors.New("not player")

// MaxUnreadAllowed Maximum number of unread messages allowed
// in orchestrator's incoming message buffer before pausing relaying
const MaxUnreadAllowed = 1e3

type messageHandler struct {
	logger zerolog.Logger

	myAddr  mino.Address
	rpc     rpc
	streams []network.Stream
	// Connects to the participants
	outs map[peer.ID]*gob.Encoder
	in   chan packet
	// Only used for the participant
	cancel context.CancelFunc
}

func (m messageHandler) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	errs := make(chan error, len(addrs))
	var wg sync.WaitGroup
	wg.Add(len(addrs))
	for _, addr := range addrs {
		go func(addr mino.Address) {
			defer wg.Done()
			err := m.send(addr, msg)
			if err != nil {
				errs <- xerrors.Errorf("could not send %T to %v: %v", msg, addr, err)
				return
			}
			m.logger.Debug().Msgf("sent %T to %v", msg, addr)
		}(addr)
	}

	go func() {
		wg.Wait()
		close(errs)
	}()
	return errs
}

func (m messageHandler) Recv(ctx context.Context) (mino.Address, serde.Message, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case packet, open := <-m.in:
		if !open {
			return nil, nil, io.EOF
		}

		origin := m.rpc.mino.GetAddressFactory().FromText(packet.Source)
		if origin == nil {
			return nil, nil, xerrors.New("could not unmarshal address")
		}
		msg, err := m.rpc.factory.Deserialize(m.rpc.context, packet.Payload)
		if err != nil {
			return origin, nil, xerrors.Errorf("could not deserialize message: %v", err)
		}

		m.logger.Debug().Msgf("received %T from %v", msg, origin)
		return origin, msg, nil
	}
}

func (m messageHandler) loop(ctx context.Context) {
	if m.isParticipant() {
		go m.waitAllClosed()
	}

	var wg sync.WaitGroup
	wg.Add(len(m.streams))
	for _, stream := range m.streams {
		go m.passMessages(ctx, stream, &wg)
	}

	go func() {
		wg.Wait()
		close(m.in)
	}()
}

func (m messageHandler) passMessages(ctx context.Context, stream network.Stream, wg *sync.WaitGroup) {
	defer wg.Done()
	decoder := gob.NewDecoder(stream)
	for {
		pkt, err := m.listen(decoder)
		if err != nil {
			if strings.Contains(err.Error(), network.ErrReset.Error()) {
				return
			}
			m.logger.Error().Err(err).Msg("message dropped")
			continue
		}
		select {
		case <-ctx.Done():
			m.logger.Trace().Msgf("messageHandler done: %v", m.isOrchestrator())
			return
		case m.in <- pkt:
		}
	}
}

func (m messageHandler) waitAllClosed() {
	for {
		for _, s := range m.streams[0].Conn().GetStreams() {
			if s.ID() == m.streams[0].ID() {
				time.Sleep(2 * time.Second)
				continue
			}
		}
		break
	}
	m.cancel()
}

func (m messageHandler) send(addr mino.Address, msg serde.Message) error {
	var unwrapped address
	switch a := addr.(type) {
	case address:
		unwrapped = a
	case orchestratorAddr:
		unwrapped = a.address
	default:
		return xerrors.Errorf("%v: %T", ErrWrongAddressType, addr)
	}

	encoder, ok := m.outs[unwrapped.identity]
	if !ok {
		return xerrors.Errorf("%v: %v", ErrNotPlayer, addr)
	}
	src, err := m.myAddr.MarshalText()
	if err != nil {
		return xerrors.Errorf("could not marshal address: %v", err)
	}
	payload, err := msg.Serialize(m.rpc.context)
	if err != nil {
		return xerrors.Errorf("could not serialize message: %v", err)
	}
	pkt := packet{Source: src, Payload: payload}

	if m.isParticipant() {
		// Send to orchestrator to relay to the destination participant
		dest, err := addr.MarshalText()
		if err != nil {
			return xerrors.Errorf("could not marshal address: %v", err)
		}
		pkt.ForwardDest = &dest
	}

	err = encoder.Encode(&pkt)
	if err != nil {
		return xerrors.Errorf("could not encode packet: %v", err)
	}
	return nil
}

func (m messageHandler) relay(packet packet, dest address) error {
	encoder, ok := m.outs[dest.identity]
	if !ok {
		return xerrors.Errorf("%v: %v", ErrNotPlayer, dest)
	}
	err := encoder.Encode(packet)
	if err != nil {
		return xerrors.Errorf("could not encode packet: %v", err)
	}
	m.logger.Debug().Stringer("to", dest).Msgf("relayed packet")
	return nil
}

func (m messageHandler) listen(decoder *gob.Decoder) (packet, error) {
	for {
		var pkt packet
		err := decoder.Decode(&pkt)
		if err != nil {
			return packet{}, xerrors.Errorf("could not decode packet: %v", err)
		}

		if m.isParticipant() {
			return pkt, nil
		}

		// An orchestrator needs to distinguish between packets for itself and packets
		// only forwarded to itself, which need to be relayed.
		if pkt.ForwardDest == nil {
			return packet{}, xerrors.Errorf("no forward found in packet for orchestrator")
		}

		dest := m.rpc.mino.GetAddressFactory().
			FromText(*pkt.ForwardDest)
		if dest == nil {
			return packet{}, xerrors.Errorf("invalid forward destination")
		}

		switch to := dest.(type) {
		case orchestratorAddr:
			if m.myAddr.Equal(to) {
				return pkt, nil
			}
		case address:
			err := m.relay(pkt, to)
			if err != nil {
				return packet{}, xerrors.Errorf("could not relay: %v", err)
			}
		}
	}
}

func (m messageHandler) isOrchestrator() bool {
	_, o := m.myAddr.(orchestratorAddr)
	return o
}

func (m messageHandler) isParticipant() bool {
	return !m.isOrchestrator()
}
