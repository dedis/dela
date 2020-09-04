package minogrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"google.golang.org/grpc/metadata"
)

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

var (
	eachLine      = regexp.MustCompile(`(?m)^(.+)$`)
	globalCounter = atomicCounter{}
	sendCounter   = &atomicCounter{}
	recvCounter   = &atomicCounter{}
	eventCounter  = &atomicCounter{}
	traffics      = []*traffic{}
	// LogItems allows one to granularly say when items should be logged or not.
	// This is useful for example in an integration test where a specific part
	// raises a problem but the full graph would be too noisy. For that, one
	// can set LogItems = false and change it to true when needed.
	LogItems = true
	// LogEvent works the same as LogItems but for events. Note that in both
	// cases the varenv should be set.
	LogEvent = true
)

// SaveItems saves all the items as a graph
func SaveItems(path string, withSend, withRcv bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	GenerateItemsGraphviz(f, withSend, withRcv, traffics...)
	return nil
}

// SaveEvents saves all the events as a graph
func SaveEvents(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	GenerateEventGraphviz(f, traffics...)
	return nil
}

// traffic is used to keep track of packets traffic in a server
type traffic struct {
	sync.Mutex
	me             mino.Address
	addressFactory mino.AddressFactory
	items          []item
	out            io.Writer
	events         []event
}

func newTraffic(me mino.Address, af mino.AddressFactory, out io.Writer) *traffic {
	traffic := &traffic{
		me:             me,
		addressFactory: af,
		items:          make([]item, 0),
		out:            out,
	}

	traffics = append(traffics, traffic)

	return traffic
}

func (t *traffic) Save(path string, withSend, withRcv bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	GenerateItemsGraphviz(f, withSend, withRcv, t)
	return nil
}

func (t *traffic) logSend(ctx context.Context, from, to mino.Address, msg router.Packet) {
	t.addItem(ctx, from, to, "send", msg)
}

// when we receive a packet, we get only the proto message, which is the
// marshalled version of the packet. The only way to have the unmarshalled
// version is by calling the "router.Forward" function, which we shouldn't do
// when we receive the packet. This is why we don't take the router.Packet.
func (t *traffic) logRcv(ctx context.Context, from, to mino.Address) {
	t.addItem(ctx, from, to, "received", nil)
}

func (t *traffic) Display(out io.Writer) {
	fmt.Fprint(out, "- traffic:\n")
	var buf bytes.Buffer
	for _, item := range t.items {
		item.Display(&buf)
	}
	fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "-$1"))
}

func (t *traffic) addItem(ctx context.Context,
	from, to mino.Address, typeStr string, msg router.Packet) {

	if t == nil || !LogItems {
		return
	}

	if to == nil {
		to = newRootAddress()
	}

	if from == nil {
		from = newRootAddress()
	}

	newItem := item{
		typeStr:       typeStr,
		msg:           msg,
		context:       t.getContext(ctx),
		globalCounter: globalCounter.IncrementAndGet(),
	}

	switch typeStr {
	case "received":
		newItem.from = from
		newItem.to = to
		newItem.typeCounter = recvCounter.IncrementAndGet()
	case "send":
		newItem.from = from
		newItem.to = to
		newItem.typeCounter = sendCounter.IncrementAndGet()
	}

	fmt.Fprintf(t.out, "\n> %v\n", t.me)
	newItem.Display(t.out)

	t.Lock()
	t.items = append(t.items, newItem)
	t.Unlock()
}

func (t *traffic) addEvent(typeStr string, from, to mino.Address) {
	if t == nil || !LogEvent {
		return
	}

	event := event{
		typeStr:       typeStr,
		from:          from,
		to:            to,
		globalCounter: eventCounter.IncrementAndGet(),
	}

	t.Lock()
	t.events = append(t.events, event)
	t.Unlock()
}

func (t *traffic) getContext(ctx context.Context) string {
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

type item struct {
	typeStr       string
	from          mino.Address
	to            mino.Address
	msg           router.Packet
	context       string
	globalCounter int
	typeCounter   int
}

func (p item) Display(out io.Writer) {
	fmt.Fprint(out, "- item:\n")
	fmt.Fprintf(out, "-- typeStr: %s\n", p.typeStr)
	fmt.Fprintf(out, "-- from: %v\n", p.from)
	fmt.Fprintf(out, "-- to: %v\n", p.to)
	fmt.Fprintf(out, "-- msg: (type %T) %s\n", p.msg, p.msg)
	if p.msg != nil {
		fmt.Fprintf(out, "--- To: %v\n", p.msg.GetDestination())
	}
	fmt.Fprintf(out, "-- context: %s\n", p.context)
}

// GenerateItemsGraphviz creates a graphviz representation of the items. One can
// generate a graphical representation with `dot -Tpdf graph.dot -o graph.pdf`
func GenerateItemsGraphviz(out io.Writer, withSend, withRcv bool, traffics ...*traffic) {

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
				item.from, item.to, item.typeCounter, item.globalCounter, msgStr, color)
		}
	}
	fmt.Fprintf(out, "}\n")
}

// GenerateEventGraphviz creates a graphviz representation of the events
func GenerateEventGraphviz(out io.Writer, traffics ...*traffic) {
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
