package qsc

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

type broadcast struct{}

func (b broadcast) send(h []epoch) (view, view, error) {
	return view{}, view{}, nil
}

type tlcrHandler struct {
	mino.UnsupportedHandler

	ch chan *Wrapper
}

func (h tlcrHandler) Process(req proto.Message) (proto.Message, error) {
	switch msg := req.(type) {
	case *Wrapper:
		h.ch <- msg
	default:
		return nil, xerrors.Errorf("invalid message type %T", req)
	}

	return nil, nil
}

type bTLCR struct {
	node     int64
	timeStep uint64
	players  mino.Players
	rpc      mino.RPC
	previous *MessageSet
	// Mino handler impl should redirect the messages to this channel..
	ch chan *Wrapper
}

func newTLCR(mino mino.Mino, players mino.Players) (*bTLCR, error) {
	ch := make(chan *Wrapper, 100)
	rpc, err := mino.MakeRPC("tlcr", tlcrHandler{ch: ch})
	if err != nil {
		return nil, err
	}

	tlcr := &bTLCR{
		timeStep: 0,
		rpc:      rpc,
		ch:       ch,
		players:  players,
		previous: nil,
	}

	return tlcr, nil
}

func (b *bTLCR) execute(message *Message) (*View, error) {
	wrapper := &Wrapper{
		Node:     b.node,
		TimeStep: b.timeStep,
		Message:  message,
		Set:      b.previous,
	}

	current := &MessageSet{
		Messages: make(map[int64]*Message),
	}

	// Broadcast the message set.
	_, errs := b.rpc.Call(wrapper, b.players)

	for len(current.GetMessages()) < b.players.Len() {
		select {
		case err := <-errs:
			fabric.Logger.Err(err).Send()
		case wrapper := <-b.ch:
			if wrapper.GetTimeStep() == b.timeStep {
				current.Messages[wrapper.GetNode()] = wrapper.GetMessage()
			} else {
				if wrapper.GetTimeStep() == b.timeStep+1 {
					for k, v := range wrapper.GetSet().GetMessages() {
						current.Messages[k] = v
					}
				}

				// Reinsert the message in the queue so that it can be processed
				// at the right time.
				b.ch <- wrapper
			}
		}
	}

	b.timeStep++
	fabric.Logger.Trace().Msgf("node %d moving to time step %d", b.node, b.timeStep)

	b.previous = current
	// TODO: implement the view.
	return &View{Received: current}, nil
}

type bTLCB struct{}
