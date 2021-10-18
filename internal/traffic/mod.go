package traffic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

// EnvVariable is the name of the environment variable to enable the traffic.
const EnvVariable = "MINO_TRAFFIC"

//
// Traffic is a utility to save network information. It can be useful for
// debugging and to understand how the network works. Each server has an
// instance of traffic but does not store logs by default. To do that you can
// set MINO_TRAFFIC=log as varenv. You can also set MINO_TRAFFIC=print to print
// the packets.
//
// There is the possibility to save a graphviz representation of network
// activity. The following snippet shows practical use of it:
//
// 	defer func() {
// 		minogrpc.SaveItems("graph.dot", true, true)
// 		minogrpc.SaveEvents("events.dot")
// 	}()
//
// Then you can generate a PDF with `dot -Tpdf graph.dot -o graph.pdf`, or
// 	MINO_TRAFFIC=log go test -run TestPedersen_Scenario && \
//		dot -Tpdf graph.dot -o graph.pdf
//

// watcherSize is the size of the watcher channels
const watcherSize = 100

var (
	eachLine      = regexp.MustCompile(`(?m)^(.+)$`)
	globalCounter = atomicCounter{}
	sendCounter   = &atomicCounter{}
	recvCounter   = &atomicCounter{}
	eventCounter  = &atomicCounter{}
	traffics      = trafficSlice{}
	// LogItems allows one to granularly say when items should be logged or not.
	// This is useful for example in an integration test where a specific part
	// raises a problem but the full graph would be too noisy. For that, one
	// can set LogItems = false and change it to true when needed.
	LogItems = true
	// LogEvent works the same as LogItems but for events. Note that in both
	// cases the varenv should be set.
	LogEvent = true

	headerURIKey = "apiuri"
)

// GlobalWatcher can be used to watch for sent and received messages.
var GlobalWatcher = Watcher{
	outWatcher: core.NewWatcher(),
	inWatcher:  core.NewWatcher(),
}

// Watcher defines an element to watch for sent and received messages.
type Watcher struct {
	outWatcher core.Observable
	inWatcher  core.Observable
}

// WatchOuts returns a channel populated with sent messages.
func (w *Watcher) WatchOuts(ctx context.Context) <-chan Event {
	return watch(ctx, w.outWatcher)
}

// WatchIns returns a channel populated with received messages.
func (w *Watcher) WatchIns(ctx context.Context) <-chan Event {
	return watch(ctx, w.inWatcher)
}

// watch is a generic function to watch for events
func watch(ctx context.Context, watcher core.Observable) <-chan Event {
	obs := observer{ch: make(chan Event, watcherSize)}

	watcher.Add(obs)

	go func() {
		<-ctx.Done()
		watcher.Remove(obs)
		close(obs.ch)
	}()

	return obs.ch
}

// SaveItems saves all the items as a graph
func SaveItems(path string, withSend, withRcv bool) error {
	f, err := os.Create(path)
	if err != nil {
		return xerrors.Errorf("file: %v", err)
	}

	GenerateItemsGraphviz(f, withSend, withRcv, traffics...)
	return nil
}

// SaveEvents saves all the events as a graph
func SaveEvents(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return xerrors.Errorf("file: %v", err)
	}

	GenerateEventGraphviz(f, traffics...)
	return nil
}

// Traffic is used to keep track of packets Traffic in a server
type Traffic struct {
	sync.Mutex
	out    io.Writer
	src    mino.Address
	items  []item
	events []event
}

// NewTraffic creates a new empty traffic recorder.
func NewTraffic(src mino.Address, out io.Writer) *Traffic {
	traffic := &Traffic{
		src:   src,
		items: make([]item, 0),
		out:   out,
	}

	traffics = append(traffics, traffic)

	return traffic
}

// Save saves the items graph to the given address.
func (t *Traffic) Save(path string, withSend, withRcv bool) error {
	f, err := os.Create(path)
	if err != nil {
		return xerrors.Errorf("file: %v", err)
	}

	GenerateItemsGraphviz(f, withSend, withRcv, t)
	return nil
}

// LogSend records a packet sent by the node. It records the node address as the
// sender and the gateway as the receiver, while also recording the packet
// itself.
func (t *Traffic) LogSend(ctx context.Context, gateway mino.Address, pkt router.Packet) {
	GlobalWatcher.outWatcher.Notify(Event{Address: gateway, Pkt: pkt})

	t.addItem(ctx, "send", gateway, pkt)
}

// LogRecv records a packet received by the node. The sender is the gateway and
// the receiver the node.
func (t *Traffic) LogRecv(ctx context.Context, gateway mino.Address, pkt router.Packet) {
	GlobalWatcher.inWatcher.Notify(Event{Address: gateway, Pkt: pkt})

	t.addItem(ctx, "received", gateway, pkt)
}

// LogRelay records a new relay.
func (t *Traffic) LogRelay(to mino.Address) {
	t.addEvent("relay", to)
}

// LogRelayClosed records the end of a relay.
func (t *Traffic) LogRelayClosed(to mino.Address) {
	t.addEvent("close", to)
}

// Display prints the current traffic to the writer.
func (t *Traffic) Display(out io.Writer) {
	t.Lock()
	defer t.Unlock()

	fmt.Fprint(out, "- traffic:\n")
	var buf bytes.Buffer
	for _, item := range t.items {
		item.Display(&buf)
	}
	fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "-$1"))
}

func (t *Traffic) addItem(ctx context.Context, typeStr string, gw mino.Address, msg router.Packet) {
	if t == nil || !LogItems {
		return
	}

	t.Lock()
	defer t.Unlock()

	newItem := item{
		src:           t.src,
		gateway:       gw,
		typeStr:       typeStr,
		msg:           msg,
		context:       t.getContext(ctx),
		globalCounter: globalCounter.IncrementAndGet(),
	}

	switch typeStr {
	case "received":
		newItem.typeCounter = recvCounter.IncrementAndGet()
	case "send":
		newItem.typeCounter = sendCounter.IncrementAndGet()
	}

	fmt.Fprintf(t.out, "\n> %v\n", t.src)
	newItem.Display(t.out)

	t.items = append(t.items, newItem)
}

func (t *Traffic) addEvent(typeStr string, to mino.Address) {
	if t == nil || !LogEvent {
		return
	}

	t.Lock()
	defer t.Unlock()

	event := event{
		typeStr:       typeStr,
		from:          t.src,
		to:            to,
		globalCounter: eventCounter.IncrementAndGet(),
	}

	fmt.Fprintf(t.out, "\n> %v\n", t.src)
	event.Display(t.out)

	t.events = append(t.events, event)
}

func (t *Traffic) getContext(ctx context.Context) string {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		headers, ok = metadata.FromOutgoingContext(ctx)
		if !ok {
			return ""
		}
	}

	values := headers.Get(headerURIKey)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func (t *Traffic) getFirstCounter() int {
	if len(t.items) > 0 {
		return t.items[0].globalCounter
	}

	if len(t.events) > 0 {
		return t.events[0].globalCounter
	}

	return math.MaxInt64
}

type item struct {
	typeStr       string
	src           mino.Address
	gateway       mino.Address
	msg           router.Packet
	context       string
	globalCounter int
	typeCounter   int
}

func (p item) Display(out io.Writer) {
	fmt.Fprint(out, "- item:\n")
	fmt.Fprintf(out, "-- typeStr: %s\n", p.typeStr)
	fmt.Fprintf(out, "-- node: %v\n", p.src)
	fmt.Fprintf(out, "-- gateway: %v\n", p.gateway)
	fmt.Fprintf(out, "-- msg: (type %T) %s\n", p.msg, p.msg)
	if p.msg != nil {
		fmt.Fprintf(out, "--- To: %v\n", p.msg.GetDestination())
	}
	fmt.Fprintf(out, "-- context: %s\n", p.context)
}

// GenerateItemsGraphviz creates a graphviz representation of the items. One can
// generate a graphical representation with `dot -Tpdf graph.dot -o graph.pdf`
func GenerateItemsGraphviz(out io.Writer, withSend, withRcv bool, traffics ...*Traffic) {

	sort.Sort(trafficSlice(traffics))

	fmt.Fprintf(out, "digraph network_activity {\n")
	fmt.Fprintf(out, "labelloc=\"t\";")
	fmt.Fprintf(out, "label = <Network Diagram of %d nodes <font point-size='10'><br/>(generated %s)</font>>;", len(traffics), time.Now().Format("2 Jan 06 - 15:04:05"))
	fmt.Fprintf(out, "graph [fontname = \"helvetica\"];")
	fmt.Fprintf(out, "graph [fontname = \"helvetica\"];")
	fmt.Fprintf(out, "node [fontname = \"helvetica\"];")
	fmt.Fprintf(out, "edge [fontname = \"helvetica\"];")

	for _, traffic := range traffics {
		for _, item := range traffic.items {

			if !withSend && item.typeStr == "send" {
				continue
			}
			if !withRcv && item.typeStr == "received" {
				continue
			}

			color := "#4AB2FF"

			if item.typeStr == "received" {
				color = "#A8A8A8"
			}

			var toStr, from, msgStr string

			if item.msg != nil {
				for _, to := range item.msg.GetDestination() {
					toStr += fmt.Sprintf("to \"%v\"<br/>", to)
				}

				from = item.msg.GetSource().String()

				msgStr = fmt.Sprintf("<font point-size='10' color='#9C9C9C'>from \"%v\"<br/>%s</font>",
					from, toStr)
			}

			fmt.Fprintf(out, "\"%v\" -> \"%v\" "+
				"[ label = < <font color='#303030'><b>%d</b> <font point-size='10'>(%d)</font></font><br/>%s> color=\"%s\" ];\n",
				item.src, item.gateway, item.typeCounter, item.globalCounter, msgStr, color)
		}
	}
	fmt.Fprintf(out, "}\n")
}

// GenerateEventGraphviz creates a graphviz representation of the events
func GenerateEventGraphviz(out io.Writer, traffics ...*Traffic) {

	sort.Sort(trafficSlice(traffics))

	fmt.Fprintf(out, "digraph network_activity {\n")
	fmt.Fprintf(out, "labelloc=\"t\";")
	fmt.Fprintf(out, "label = <Network Diagram of %d nodes <font point-size='10'><br/>(generated %s)</font>>;", len(traffics), time.Now().Format("2 Jan 06 - 15:04:05"))
	fmt.Fprintf(out, "graph [fontname = \"helvetica\"];")
	fmt.Fprintf(out, "graph [fontname = \"helvetica\"];")
	fmt.Fprintf(out, "node [fontname = \"helvetica\"];")
	fmt.Fprintf(out, "edge [fontname = \"helvetica\"];")

	for _, t := range traffics {
		for _, event := range t.events {

			msgStr := event.typeStr

			color := "#4AB2FF"

			if event.typeStr == "close" {
				color = "#A8A8A8"
			}

			fmt.Fprintf(out, "\"%v\" -> \"%v\" "+
				"[ label = < <font color='#303030'><b>%d</b></font><br/>%s> color=\"%s\" ];\n",
				event.from, event.to, event.globalCounter, msgStr, color)
		}
	}

	fmt.Fprintf(out, "}\n")
}

// This counter only makes sense when all the nodes are running locally. It is
// useful to analyse the traffic in a developping/test environment, when packets
// order makes sense.
type atomicCounter struct {
	sync.Mutex
	c int
}

func (c *atomicCounter) IncrementAndGet() int {
	c.Lock()
	defer c.Unlock()
	c.c++
	return c.c
}

type event struct {
	typeStr       string
	from          mino.Address
	to            mino.Address
	globalCounter int
}

func (e event) Display(out io.Writer) {
	fmt.Fprintf(out, "%s\t%s:\n", e.typeStr, e.to)
}

type trafficSlice []*Traffic

func (tt trafficSlice) Len() int {
	return len(tt)
}

func (tt trafficSlice) Less(i, j int) bool {
	return tt[i].getFirstCounter() < tt[j].getFirstCounter()
}

func (tt trafficSlice) Swap(i, j int) {
	tt[i], tt[j] = tt[j], tt[i]
}

// observer defines an observer that fills a channel to notify.
//
// - implements core.Observer
type observer struct {
	ch chan Event
}

// NotifyCallback implements core.Observer. It drops the message if the channel
// is full.
func (o observer) NotifyCallback(event interface{}) {
	select {
	case o.ch <- event.(Event):
	default:
		dela.Logger.Warn().Msg("event channel full, dropping")
	}
}

// Event defines the elements of a receive or sent event
type Event struct {
	Address mino.Address
	Pkt     router.Packet
}
