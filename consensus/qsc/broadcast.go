package qsc

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/mino"
)

type storage struct {
	previous *MessageSet
}

type broadcast struct{}

func (b broadcast) send(prop consensus.Proposal) (view, view, error) {
	return view{}, view{}, nil
}

type hTLCR struct {
	mino.UnsupportedHandler

	ch    chan *MessageSet
	store *storage
}

func (h hTLCR) Process(in proto.Message) (proto.Message, error) {
	switch msg := in.(type) {
	case *MessageSet:
		h.ch <- msg
	case *RequestMessageSet:
		if msg.GetTimeStep() != h.store.previous.GetTimeStep() {
			return nil, nil
		}

		resp := proto.Clone(h.store.previous).(*MessageSet)
		for _, node := range msg.GetNodes() {
			// Remove those not necessary to reduce the message size.
			delete(resp.Messages, node)
		}

		return h.store.previous, nil
	}

	return nil, nil
}

type bTLCR struct {
	node     int64
	timeStep uint64
	players  mino.Players
	rpc      mino.RPC
	store    *storage
	// Mino handler impl should redirect the messages to this channel..
	ch chan *MessageSet
}

func newTLCR(mino mino.Mino, players mino.Players) (*bTLCR, error) {
	ch := make(chan *MessageSet, 100)
	store := &storage{}
	rpc, err := mino.MakeRPC("tlcr", hTLCR{ch: ch, store: store})
	if err != nil {
		return nil, err
	}

	tlcr := &bTLCR{
		timeStep: 0,
		rpc:      rpc,
		ch:       ch,
		players:  players,
		store:    store,
	}

	return tlcr, nil
}

func (b *bTLCR) processEpoch(curr *MessageSet, other *MessageSet) error {
	if other.GetTimeStep() == b.timeStep {
		// Merge both message set.
		for k, v := range other.GetMessages() {
			fabric.Logger.Trace().Msgf("%d adding a message from %d for time step %d",
				b.node, k, b.timeStep)

			curr.Messages[k] = v
		}
	} else {
		// Reinsert the message in the queue so that it can be processed
		// at the right time.
		b.ch <- other

		if other.GetTimeStep() == b.timeStep+1 {
			fabric.Logger.Debug().Msgf("%d requesting previous message set for time step %d", b.node, b.timeStep)
			req := &RequestMessageSet{
				TimeStep: b.timeStep,
				Nodes:    make([]int64, 0, len(curr.GetMessages())),
			}

			// It sends the messages that it already has so that the distant
			// player can send back the missing ones.
			for node := range curr.GetMessages() {
				req.Nodes = append(req.Nodes, node)
			}

			previous, err := b.requestPreviousSet(int(other.GetNode()), req)
			if err != nil {
				return err
			}

			fabric.Logger.Debug().Msgf("filling missing %d messages", len(previous.GetMessages()))

			for k, v := range previous.GetMessages() {
				curr.Messages[k] = v
			}
		}
	}

	return nil
}

func (b *bTLCR) requestPreviousSet(node int, req *RequestMessageSet) (*MessageSet, error) {
	resps, errs := b.rpc.Call(req, b.players.SubSet(node, node+1))
	select {
	case resp := <-resps:
		ms, ok := resp.(*MessageSet)
		if ok {
			return ms, nil
		}
	case err := <-errs:
		fabric.Logger.Err(err).Send()
	}

	return nil, nil
}

func (b *bTLCR) execute(messages ...*Message) (*View, error) {
	ms := &MessageSet{
		Messages: make(map[int64]*Message),
		TimeStep: b.timeStep,
		Node:     b.node,
	}
	for _, msg := range messages {
		ms.Messages[msg.GetNode()] = msg
	}

	_, errs := b.rpc.Call(ms, b.players)

	for len(ms.GetMessages()) < b.players.Len()-1 {
		select {
		case err := <-errs:
			fabric.Logger.Err(err).Send()
		case req := <-b.ch:
			err := b.processEpoch(ms, req)
			if err != nil {
				return nil, err
			}
		}
	}

	b.timeStep++
	fabric.Logger.Trace().Msgf("node %d moving to time step %d", b.node, b.timeStep)

	b.store.previous = ms
	// TODO: implement the view.
	return &View{Received: ms}, nil
}

type bTLCB struct {
	b1 *bTLCR
	b2 *bTLCR
}

func (b *bTLCB) execute(prop consensus.Proposal) (*View, error) {
	return &View{}, nil
}
