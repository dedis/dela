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

	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/dela/mino"
	"google.golang.org/grpc/metadata"
)

//
// Traffic is a utility to save network informations. It can be useful for
// debugging and to understand how the network works. Each server has an
// instance of traffic but does not store logs by default. To do that you can
// set MINO_TRAFFIC=log as varenv. You can also set MINO_TRAFFIC=print to print
// the packets.
//
// There is the possibility to save a graphviz representation of network
// activity. The following snippet shows practical use of it:
//
// ```go minogrpc.SaveLog = true defer func() {
//     minogrpc.SaveGraph("graph.dot", true, false)
// }() ```
//
// Then you can generate of PDF with `dot -Tpdf graph.dot -o graph.pdf`
//

var (
	eachLine      = regexp.MustCompile(`(?m)^(.+)$`)
	globalCounter = atomicCounter{}
	sendCounter   = &atomicCounter{}
	recvCounter   = &atomicCounter{}
)

// traffic is used to keep track of packets traffic in a server
type traffic struct {
	me             mino.Address
	addressFactory mino.AddressFactory
	items          []item
	out            io.Writer
}

func newTraffic(me mino.Address, af mino.AddressFactory, out io.Writer) *traffic {
	traffic := &traffic{
		me:             me,
		addressFactory: af,
		items:          make([]item, 0),
		out:            out,
	}

	return traffic
}

func (t *traffic) Save(path string, withSend, withRcv bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	GenerateGraphviz(f, withSend, withRcv, t)
	return nil
}

func (t *traffic) logSend(ctx context.Context, from, to mino.Address, msg *Envelope) {
	t.addItem(ctx, from, to, "send", msg)
}

func (t *traffic) logRcv(ctx context.Context, from, to mino.Address, msg *Envelope) {
	t.addItem(ctx, from, to, "received", msg)
}

func (t traffic) Display(out io.Writer) {
	fmt.Fprint(out, "- traffic:\n")
	var buf bytes.Buffer
	for _, item := range t.items {
		item.Display(&buf)
	}
	fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "-$1"))
}

func (t *traffic) addItem(ctx context.Context,
	from, to mino.Address, typeStr string, msg *Envelope) {

	if t == nil {
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

	fmt.Fprintf(t.out, "\n> %v", t.me)
	newItem.Display(t.out)

	t.items = append(t.items, newItem)
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
	msg           *Envelope
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
	fmt.Fprintf(out, "--- To: %v\n", p.msg.To)
	fmt.Fprintf(out, "-- context: %s\n", p.context)
}

// GenerateGraphviz creates a graphviz representation. One can generate a
// graphical representation with `dot -Tpdf graph.dot -o graph.pdf`
func GenerateGraphviz(out io.Writer, withSend, withRcv bool, traffics ...*traffic) {

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

			msgType := fmt.Sprintf("%T", item.msg.Message)
			var da ptypes.DynamicAny
			err := ptypes.UnmarshalAny(item.msg.Message.GetPayload(), &da)
			if err == nil {
				msgType = fmt.Sprintf("%T", da.Message)
			}

			color := "#4AB2FF"

			if item.typeStr == "received" {
				color = "#A8A8A8"
			}

			toStr := ""
			for _, to := range item.msg.To {
				addr := traffic.addressFactory.FromText(to)
				toStr += fmt.Sprintf("to \"%v\"<br/>", addr)
			}

			from := traffic.addressFactory.FromText(item.msg.GetMessage().From)

			msgStr := fmt.Sprintf("<font point-size='10' color='#9C9C9C'>from \"%v\"<br/>%s%s</font>",
				from, toStr, msgType)

			fmt.Fprintf(out, "\"%v\" -> \"%v\" "+
				"[ label = < <font color='#303030'><b>%d</b> <font point-size='10'>(%d)</font></font><br/>%s> color=\"%s\" ];\n",
				item.from, item.to, item.typeCounter, item.globalCounter, msgStr, color)
		}
	}
	fmt.Fprintf(out, "}\n")
}

// This coutner only makes sense when all the nodes are running locally. It is
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
