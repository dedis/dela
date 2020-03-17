package qsc

import (
	"context"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/rs/zerolog"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// storage is a shared storage object between the handler and the
// implementation.
type storage struct {
	previous *MessageSet
}

type broadcast interface {
	send(context.Context, history) (*View, error)
}

// broadcastTCLB implements the Threshold Synchronous Broadcast primitive necessary
// to implement the Que Sera Consensus.
type broadcastTCLB struct {
	*bTLCB
}

// newBroadcast returns a TSB primitive that is using TLCB for the underlying
// implementation.
// TODO: implement TLCWR
func newBroadcast(node int64, mino mino.Mino, players mino.Players) (broadcastTCLB, error) {
	bc := broadcastTCLB{}

	tlcb, err := newTLCB(node, mino, players)
	if err != nil {
		return bc, err
	}

	bc.bTLCB = tlcb
	return bc, nil
}

// send broadcasts the history for the current time step and move to the next
// one.
func (b broadcastTCLB) send(ctx context.Context, h history) (*View, error) {
	packed, err := h.Pack()
	if err != nil {
		return nil, err
	}

	return b.execute(ctx, packed)
}

// hTLCR is the Mino handler for the TLCR implementation.
//
// - implements mino.Handler
type hTLCR struct {
	mino.UnsupportedHandler

	ch    chan *MessageSet
	store *storage
}

// Process implements mino.Handler. It handles two cases: (1) A message set sent
// from a player that must be processed. (2) A message set request that returns
// the list of messages missing to the distant player.
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

type tlcr interface {
	execute(context.Context, ...*Message) (*View, error)
}

// bTLCR implements the TLCR primitive from the QSC paper.
type bTLCR struct {
	logger   zerolog.Logger
	node     int64
	timeStep uint64
	players  mino.Players
	rpc      mino.RPC
	store    *storage
	// Mino handler impl should redirect the messages to this channel..
	ch chan *MessageSet
}

func newTLCR(name string, node int64, mino mino.Mino, players mino.Players) (*bTLCR, error) {
	// TODO: improve to have a buffer per node with limited size.
	ch := make(chan *MessageSet, 1000)
	store := &storage{}
	rpc, err := mino.MakeRPC(name, hTLCR{ch: ch, store: store})
	if err != nil {
		return nil, err
	}

	tlcr := &bTLCR{
		logger:   fabric.Logger,
		node:     node,
		timeStep: 0,
		rpc:      rpc,
		ch:       ch,
		players:  players,
		store:    store,
	}

	return tlcr, nil
}

func (b *bTLCR) execute(ctx context.Context, messages ...*Message) (*View, error) {
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
			b.logger.Err(err).Msg("couldn't broadcast to everyone")
		case req := <-b.ch:
			if req.GetTimeStep() == b.timeStep {
				b.merge(ms, req)
			} else {
				err := b.catchUp(ms, req)
				if err != nil {
					b.logger.Err(err).Send()
				}
			}
		case <-ctx.Done():
			return nil, xerrors.Errorf("context is done: %v", ctx.Err())
		}
	}

	b.timeStep++
	b.logger.Trace().Msgf("node %d moving to time step %d", b.node, b.timeStep)

	b.store.previous = ms

	return &View{Received: ms.GetMessages()}, nil
}

// merge merges received into curr.
func (b *bTLCR) merge(current, received *MessageSet) {
	for k, v := range received.GetMessages() {
		current.Messages[k] = v
	}
}

// catchUp analyses the received message set compared to the current one and
// tries to catch up if possible, otherwise it delays the processing by
// inserting the message at the end of the queue.
func (b *bTLCR) catchUp(current, received *MessageSet) error {
	// Reinsert the message in the queue so that it can be processed
	// at the right time.
	b.ch <- received

	if received.GetTimeStep() == b.timeStep+1 {
		// The message is only one step further than the current time step so we
		// can use the previous message set to catch up and move forward.
		b.logger.Trace().Msgf("%d requesting previous message set for time step %d", b.node, b.timeStep)

		req := &RequestMessageSet{
			TimeStep: b.timeStep,
			Nodes:    make([]int64, 0, len(current.GetMessages())),
		}

		// It sends the messages that it already has so that the distant
		// player can send back the missing ones.
		for node := range current.GetMessages() {
			req.Nodes = append(req.Nodes, node)
		}

		previous, err := b.requestPreviousSet(int(received.GetNode()), req)
		if err != nil {
			return xerrors.Errorf("couldn't fetch previous message set: %v", err)
		}

		b.logger.Trace().Msgf("filling missing %d messages", len(previous.GetMessages()))

		for k, v := range previous.GetMessages() {
			current.Messages[k] = v
		}
	}

	return nil
}

func (b *bTLCR) requestPreviousSet(node int, req *RequestMessageSet) (*MessageSet, error) {
	resps, errs := b.rpc.Call(req, b.players.Take(mino.FilterIndex(node)))
	select {
	case resp, ok := <-resps:
		if !ok {
			return nil, xerrors.New("couldn't get a reply")
		}

		ms, ok := resp.(*MessageSet)
		if !ok {
			return nil, xerrors.Errorf("invalid message type: %T", resp)
		}

		return ms, nil
	case err := <-errs:
		return nil, xerrors.Errorf("couldn't reach the node: %v", err)
	}
}

type bTLCB struct {
	node            int64
	spreadThreshold int
	b1              tlcr
	b2              tlcr
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
		node:            node,
		spreadThreshold: players.Len(),
	}, nil
}

func (b *bTLCB) execute(ctx context.Context, message proto.Message) (*View, error) {
	m, err := b.makeMessage(message)
	if err != nil {
		return nil, err
	}

	fabric.Logger.Trace().Msgf("%d going through prepare broadcast", b.node)
	prepareSet, err := b.b1.execute(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't broadcast: %v", err)
	}

	m2, err := b.makeMessage(&MessageSet{Messages: prepareSet.GetReceived()})
	if err != nil {
		return nil, err
	}

	fabric.Logger.Trace().Msgf("%d going through commit broadcast", b.node)
	commitSet, err := b.b2.execute(ctx, m2)
	if err != nil {
		return nil, xerrors.Errorf("couldn't broadcast: %v", err)
	}

	ret, err := b.merge(prepareSet, commitSet)
	if err != nil {
		return nil, xerrors.Errorf("couldn't merge: %v", err)
	}

	return ret, nil
}

func (b *bTLCB) makeMessage(message proto.Message) (*Message, error) {
	value, err := protoenc.MarshalAny(message)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(message, err)
	}

	m := &Message{
		Node:  b.node,
		Value: value,
	}

	return m, nil
}

func (b *bTLCB) merge(prepare, commit *View) (*View, error) {
	ret := &View{
		Received:    make(map[int64]*Message),
		Broadcasted: make(map[int64]*Message),
	}

	for node, msg := range prepare.GetReceived() {
		ret.Received[node] = msg
	}

	counter := make(map[int64]int)
	for _, msproto := range commit.GetReceived() {
		ms := &MessageSet{}
		err := ptypes.UnmarshalAny(msproto.GetValue(), ms)
		if err != nil {
			return nil, encoding.NewAnyDecodingError(ms, err)
		}

		for node, msg := range ms.GetMessages() {
			// Populate the received set anyway.
			ret.Received[node] = msg
			// The broadcasted set is filled and later on cleaned according to
			// the spread threshold.
			if _, ok := counter[node]; !ok {
				counter[node] = 0
				ret.Broadcasted[node] = msg
			}
			counter[node]++
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
