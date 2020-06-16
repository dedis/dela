package qsc

import (
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// storage is a shared storage object between the handler and the
// implementation.
type storage struct {
	previous MessageSet
}

// View defines a round result with received and broadcasted messages.
type View struct {
	received    map[int64]Message
	broadcasted map[int64]Message
}

type broadcast interface {
	send(context.Context, history) (View, error)
}

// broadcastTCLB implements the Threshold Synchronous Broadcast primitive
// necessary to implement the Que Sera Consensus.
type broadcastTCLB struct {
	*bTLCB
}

// newBroadcast returns a TSB primitive that is using TLCB for the underlying
// implementation.
// TODO: implement TLCWR
func newBroadcast(node int64, mino mino.Mino, players mino.Players, f serde.Factory) (broadcastTCLB, error) {
	bc := broadcastTCLB{}

	tlcb, err := newTLCB(node, mino, players, f)
	if err != nil {
		return bc, xerrors.Errorf("couldn't make TLCB: %v", err)
	}

	bc.bTLCB = tlcb
	return bc, nil
}

// send broadcasts the history for the current time step and move to the next
// one.
func (b broadcastTCLB) send(ctx context.Context, h history) (View, error) {
	return b.execute(ctx, h)
}

// hTLCR is the Mino handler for the TLCR implementation.
//
// - implements mino.Handler
type hTLCR struct {
	mino.UnsupportedHandler

	ch    chan MessageSet
	store *storage
}

// Process implements mino.Handler. It handles two cases: (1) A message set sent
// from a player that must be processed. (2) A message set request that returns
// the list of messages missing to the distant player.
func (h hTLCR) Process(req mino.Request) (serde.Message, error) {
	switch msg := req.Message.(type) {
	case MessageSet:
		h.ch <- msg
		return nil, nil
	case RequestMessageSet:
		if msg.timeStep != h.store.previous.timeStep {
			return nil, nil
		}

		resp := h.store.previous
		resp.messages = make(map[int64]Message)

		for _, node := range msg.nodes {
			resp.messages[node] = h.store.previous.messages[node]
		}

		return resp, nil
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", req.Message)
	}
}

type tlcr interface {
	execute(context.Context, ...Message) (View, error)
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
	ch chan MessageSet
}

func newTLCR(name string, node int64, mino mino.Mino, players mino.Players, f serde.Factory) (*bTLCR, error) {
	// TODO: improve to have a buffer per node with limited size.
	handler := hTLCR{
		ch:    make(chan MessageSet, 1000),
		store: &storage{},
	}

	rpc, err := mino.MakeRPC(name, handler, RequestFactory{mFactory: f})
	if err != nil {
		return nil, err
	}

	tlcr := &bTLCR{
		logger:   dela.Logger,
		node:     node,
		timeStep: 0,
		rpc:      rpc,
		ch:       handler.ch,
		players:  players,
		store:    handler.store,
	}

	return tlcr, nil
}

func (b *bTLCR) execute(ctx context.Context, messages ...Message) (View, error) {
	ms := MessageSet{
		messages: make(map[int64]Message),
		timeStep: b.timeStep,
		node:     b.node,
	}
	for _, msg := range messages {
		ms.messages[msg.node] = msg
	}

	_, errs := b.rpc.Call(ctx, ms, b.players)

	for len(ms.messages) < b.players.Len() {
		select {
		case err := <-errs:
			b.logger.Err(err).Msg("couldn't broadcast to everyone")
		case req := <-b.ch:
			if req.timeStep == b.timeStep {
				b.merge(ms, req)
			} else {
				err := b.catchUp(ctx, ms, req)
				if err != nil {
					b.logger.Err(err).Send()
				}
			}
		case <-ctx.Done():
			return View{}, ctx.Err()
		}
	}

	b.timeStep++
	b.logger.Trace().Msgf("node %d moving to time step %d", b.node, b.timeStep)

	b.store.previous = ms

	return View{received: ms.messages}, nil
}

// merge merges messages from received into current.
func (b *bTLCR) merge(current, received MessageSet) {
	for k, v := range received.messages {
		current.messages[k] = v
	}
}

// catchUp analyses the messageSet 'received' compared to 'current' and tries to
// catch up if possible, otherwise it delays the processing by inserting the
// message at the end of the queue.
func (b *bTLCR) catchUp(ctx context.Context, current, received MessageSet) error {
	// Reinserts the messageSet in the queue so that it can be processed at the
	// right time.
	b.ch <- received

	if received.timeStep == b.timeStep+1 {
		// The messageSet is only one step further than the current time step so
		// we can use the previous messageSet to catch up and move forward.
		b.logger.Trace().Msgf("%d requesting previous message set for time step %d", b.node, b.timeStep)

		req := RequestMessageSet{
			timeStep: b.timeStep,
			nodes:    make([]int64, 0, len(current.messages)),
		}

		// It sends the messages that it already has so that the distant
		// player can send back the missing ones.
		for node := range current.messages {
			req.nodes = append(req.nodes, node)
		}

		previous, err := b.requestPreviousSet(ctx, int(received.node), req)
		if err != nil {
			return xerrors.Errorf("couldn't fetch previous message set: %w", err)
		}

		b.logger.Trace().Msgf("filling missing %d messages", len(previous.messages))

		for k, v := range previous.messages {
			current.messages[k] = v
		}
	}

	return nil
}

func (b *bTLCR) requestPreviousSet(ctx context.Context, node int,
	req RequestMessageSet) (MessageSet, error) {

	resps, errs := b.rpc.Call(ctx, req, b.players.Take(mino.IndexFilter(node)))
	select {
	case resp, ok := <-resps:
		if !ok {
			return MessageSet{}, xerrors.New("couldn't get a reply")
		}

		ms, ok := resp.(MessageSet)
		if !ok {
			return MessageSet{}, xerrors.Errorf("got message type '%T' but expected '%T'",
				resp, ms)
		}

		return ms, nil
	case err := <-errs:
		return MessageSet{}, xerrors.Errorf("couldn't reach the node: %v", err)
	}
}

type bTLCB struct {
	node            int64
	spreadThreshold int
	b1              tlcr
	b2              tlcr
}

func newTLCB(node int64, mino mino.Mino, players mino.Players, f serde.Factory) (*bTLCB, error) {
	b1, err := newTLCR("tlcr-prepare", node, mino, players, f)
	if err != nil {
		return nil, err
	}
	b2, err := newTLCR("tlcr-commit", node, mino, players, f)
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

func (b *bTLCB) execute(ctx context.Context, value serde.Message) (View, error) {
	m := Message{
		node:  b.node,
		value: value,
	}

	dela.Logger.Trace().Msgf("%d going through prepare broadcast", b.node)
	prepareSet, err := b.b1.execute(ctx, m)
	if err != nil {
		return View{}, xerrors.Errorf("couldn't broadcast: %v", err)
	}

	m2 := Message{
		node:  b.node,
		value: MessageSet{messages: prepareSet.received},
	}

	dela.Logger.Trace().Msgf("%d going through commit broadcast", b.node)
	commitSet, err := b.b2.execute(ctx, m2)
	if err != nil {
		return View{}, xerrors.Errorf("couldn't broadcast: %v", err)
	}

	ret, err := b.merge(prepareSet, commitSet)
	if err != nil {
		return View{}, xerrors.Errorf("couldn't merge: %v", err)
	}

	return ret, nil
}

func (b *bTLCB) merge(prepare, commit View) (View, error) {
	ret := View{
		received:    make(map[int64]Message),
		broadcasted: make(map[int64]Message),
	}

	for node, msg := range prepare.received {
		ret.received[node] = msg
	}

	counter := make(map[int64]int)
	for _, msg := range commit.received {
		mset := msg.value.(MessageSet)

		for node, msg := range mset.messages {
			// Populate the received set anyway.
			ret.received[node] = msg
			// The broadcasted set is filled and later on cleaned according to
			// the spread threshold.
			_, found := counter[node]
			if !found {
				counter[node] = 0
				ret.broadcasted[node] = msg
			}
			counter[node]++
		}
	}

	// Clean the broadcasted set
	for node, sum := range counter {
		if sum < b.spreadThreshold {
			delete(ret.broadcasted, node)
		}
	}

	return ret, nil
}
