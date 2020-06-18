package qsc

import (
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/qsc/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

// storage is a shared storage object between the handler and the
// implementation.
type storage struct {
	previous types.MessageSet
}

// View defines a round result with received and broadcasted messages.
type View struct {
	received    map[int64]types.Message
	broadcasted map[int64]types.Message
}

func NewReceiveView(msgs []types.Message) View {
	received := make(map[int64]types.Message)
	for _, msg := range msgs {
		received[msg.GetNode()] = msg
	}

	return View{received: received}
}

func (v View) GetReceived() []types.Message {
	msgs := make([]types.Message, 0, len(v.received))
	for _, msg := range v.received {
		msgs = append(msgs, msg)
	}

	return msgs
}

type broadcast interface {
	send(context.Context, types.History) (View, error)
}

// broadcastTCLB implements the Threshold Synchronous Broadcast primitive
// necessary to implement the Que Sera Consensus.
type broadcastTCLB struct {
	*bTLCB
}

// newBroadcast returns a TSB primitive that is using TLCB for the underlying
// implementation.
// TODO: implement TLCWR
func newBroadcast(node int64, mino mino.Mino, players mino.Players, f serdeng.Factory) (broadcastTCLB, error) {
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
func (b broadcastTCLB) send(ctx context.Context, h types.History) (View, error) {
	return b.execute(ctx, h)
}

// hTLCR is the Mino handler for the TLCR implementation.
//
// - implements mino.Handler
type hTLCR struct {
	mino.UnsupportedHandler

	ch    chan types.MessageSet
	store *storage
}

// Process implements mino.Handler. It handles two cases: (1) A message set sent
// from a player that must be processed. (2) A message set request that returns
// the list of messages missing to the distant player.
func (h hTLCR) Process(req mino.Request) (serdeng.Message, error) {
	switch msg := req.Message.(type) {
	case types.MessageSet:
		h.ch <- msg
		return nil, nil
	case types.RequestMessageSet:
		if msg.GetTimeStep() != h.store.previous.GetTimeStep() {
			return nil, nil
		}

		resp := h.store.previous.Reduce(msg.GetNodes())

		return resp, nil
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", req.Message)
	}
}

type tlcr interface {
	execute(context.Context, ...types.Message) (View, error)
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
	ch chan types.MessageSet
}

func newTLCR(name string, node int64, mino mino.Mino, players mino.Players, f serdeng.Factory) (*bTLCR, error) {
	// TODO: improve to have a buffer per node with limited size.
	handler := hTLCR{
		ch:    make(chan types.MessageSet, 1000),
		store: &storage{},
	}

	rpc, err := mino.MakeRPC(name, handler, types.NewRequestFactory(f))
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

func (b *bTLCR) execute(ctx context.Context, messages ...types.Message) (View, error) {
	ms := types.NewMessageSet(b.node, b.timeStep, messages...)

	_, errs := b.rpc.Call(ctx, ms, b.players)

	for len(ms.GetMessages()) < b.players.Len() {
		select {
		case err := <-errs:
			b.logger.Err(err).Msg("couldn't broadcast to everyone")
		case req := <-b.ch:
			if req.GetTimeStep() == b.timeStep {
				ms = ms.Merge(req)
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

	return NewReceiveView(ms.GetMessages()), nil
}

// catchUp analyses the messageSet 'received' compared to 'current' and tries to
// catch up if possible, otherwise it delays the processing by inserting the
// message at the end of the queue.
func (b *bTLCR) catchUp(ctx context.Context, current, received types.MessageSet) error {
	// Reinserts the messageSet in the queue so that it can be processed at the
	// right time.
	b.ch <- received

	if received.GetTimeStep() == b.timeStep+1 {
		// The messageSet is only one step further than the current time step so
		// we can use the previous messageSet to catch up and move forward.
		b.logger.Trace().Msgf("%d requesting previous message set for time step %d", b.node, b.timeStep)

		nodes := make([]int64, 0, len(current.GetMessages()))

		// It sends the messages that it already has so that the distant
		// player can send back the missing ones.
		for _, msg := range current.GetMessages() {
			nodes = append(nodes, msg.GetNode())
		}

		req := types.NewRequestMessageSet(b.timeStep, nodes)

		previous, err := b.requestPreviousSet(ctx, int(received.GetNode()), req)
		if err != nil {
			return xerrors.Errorf("couldn't fetch previous message set: %w", err)
		}

		b.logger.Trace().Msgf("filling missing %d messages", len(previous.GetMessages()))

		current = previous.Merge(current)
	}

	return nil
}

func (b *bTLCR) requestPreviousSet(ctx context.Context, node int,
	req types.RequestMessageSet) (types.MessageSet, error) {

	resps, errs := b.rpc.Call(ctx, req, b.players.Take(mino.IndexFilter(node)))
	select {
	case resp, ok := <-resps:
		if !ok {
			return types.MessageSet{}, xerrors.New("couldn't get a reply")
		}

		ms, ok := resp.(types.MessageSet)
		if !ok {
			return types.MessageSet{}, xerrors.Errorf("got message type '%T' but expected '%T'",
				resp, ms)
		}

		return ms, nil
	case err := <-errs:
		return types.MessageSet{}, xerrors.Errorf("couldn't reach the node: %v", err)
	}
}

type bTLCB struct {
	node            int64
	spreadThreshold int
	b1              tlcr
	b2              tlcr
}

func newTLCB(node int64, mino mino.Mino, players mino.Players, f serdeng.Factory) (*bTLCB, error) {
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

func (b *bTLCB) execute(ctx context.Context, value serdeng.Message) (View, error) {
	m := types.NewMessage(b.node, value)

	dela.Logger.Trace().Msgf("%d going through prepare broadcast", b.node)
	prepareSet, err := b.b1.execute(ctx, m)
	if err != nil {
		return View{}, xerrors.Errorf("couldn't broadcast: %v", err)
	}

	mset := types.NewMessageSet(b.node, 0, prepareSet.GetReceived()...)

	m2 := types.NewMessage(b.node, mset)

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
		received:    make(map[int64]types.Message),
		broadcasted: make(map[int64]types.Message),
	}

	for node, msg := range prepare.received {
		ret.received[node] = msg
	}

	counter := make(map[int64]int)
	for _, msg := range commit.received {
		mset := msg.GetValue().(types.MessageSet)

		for _, msg := range mset.GetMessages() {
			node := msg.GetNode()

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
