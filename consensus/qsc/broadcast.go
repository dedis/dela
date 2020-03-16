package qsc

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
)

type storage struct {
	previous *MessageSet
}

type broadcast struct {
	*bTLCB
}

func newBroadcast(node int64, mino mino.Mino, players mino.Players) (broadcast, error) {
	bc := broadcast{}

	tlcb, err := newTLCB(node, mino, players)
	if err != nil {
		return bc, err
	}

	bc.bTLCB = tlcb
	return bc, nil
}

func (b broadcast) send(h history) (*View, error) {
	packed, err := h.Pack()
	if err != nil {
		return nil, err
	}

	return b.execute(packed)
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

func newTLCR(name string, node int64, mino mino.Mino, players mino.Players) (*bTLCR, error) {
	ch := make(chan *MessageSet, 100)
	store := &storage{}
	rpc, err := mino.MakeRPC(name, hTLCR{ch: ch, store: store})
	if err != nil {
		return nil, err
	}

	tlcr := &bTLCR{
		node:     node,
		timeStep: 0,
		rpc:      rpc,
		ch:       ch,
		players:  players,
		store:    store,
	}

	return tlcr, nil
}

func (b *bTLCR) processMessageSet(curr *MessageSet, other *MessageSet) error {
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
			fabric.Logger.Trace().Msgf("%d requesting previous message set for time step %d", b.node, b.timeStep)
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

			fabric.Logger.Trace().Msgf("filling missing %d messages", len(previous.GetMessages()))

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

	for len(ms.GetMessages()) < b.players.Len() {
		select {
		case err := <-errs:
			fabric.Logger.Err(err).Send()
		case req := <-b.ch:
			err := b.processMessageSet(ms, req)
			if err != nil {
				return nil, err
			}
		}
	}

	b.timeStep++
	fabric.Logger.Trace().Msgf("node %d moving to time step %d", b.node, b.timeStep)

	b.store.previous = ms
	// TODO: implement the view.
	return &View{Received: ms.GetMessages()}, nil
}

type bTLCB struct {
	spreadThreshold int
	b1              *bTLCR
	b2              *bTLCR
}

func newTLCB(node int64, mino mino.Mino, players mino.Players) (*bTLCB, error) {
	b1, err := newTLCR("tlcr-prepare", node, mino, players)
	if err != nil {
		return nil, err
	}
	b2, err := newTLCR("tlcr-commit", node, mino, players)
	if err != nil {
		return nil, err
	}

	return &bTLCB{
		b1:              b1,
		b2:              b2,
		spreadThreshold: players.Len(),
	}, nil
}

func (b *bTLCB) execute(message proto.Message) (*View, error) {
	value, err := ptypes.MarshalAny(message)
	if err != nil {
		return nil, err
	}

	m := &Message{
		Node:  b.b1.node,
		Value: value,
	}

	fabric.Logger.Trace().Msgf("%d going through prepare broadcast", b.b1.node)
	view, err := b.b1.execute(m)
	if err != nil {
		return nil, err
	}

	receivedAny, err := ptypes.MarshalAny(&MessageSet{Messages: view.GetReceived()})
	if err != nil {
		return nil, err
	}

	m2 := &Message{
		Node:  b.b2.node,
		Value: receivedAny,
	}

	fabric.Logger.Trace().Msgf("%d going through commit broadcast", b.b1.node)
	view2, err := b.b2.execute(m2)
	if err != nil {
		return nil, err
	}

	ret := &View{
		Received:    make(map[int64]*Message),
		Broadcasted: make(map[int64]*Message),
	}

	// Merge received sets.
	for k, v := range view.GetReceived() {
		ret.Received[k] = v
	}
	counter := make(map[int64]int)
	for _, msg := range view2.GetReceived() {
		ms := &MessageSet{}
		err := ptypes.UnmarshalAny(msg.GetValue(), ms)
		if err != nil {
			return nil, err
		}

		for k, v := range ms.GetMessages() {
			ret.Received[k] = v
			if _, ok := counter[k]; !ok {
				counter[k] = 0
				ret.Broadcasted[k] = v
			}
			counter[k]++
		}
	}

	// Clean the broadcasted set
	for node, sum := range counter {
		if sum < b.spreadThreshold {
			delete(ret.Broadcasted, node)
		}
	}

	return ret, nil
}
