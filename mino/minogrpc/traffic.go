package minogrpc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
)

var (
	eachLine      = regexp.MustCompile(`(?m)^(.+)$`)
	globalCounter = atomicCounter{}
	sendCounter   = &atomicCounter{}
	recvCounter   = &atomicCounter{}
	traffics      = []*traffic{}
)

// SaveGraph generate the graph file based on the saved traffics. Be sure to set
// MINO_LOG_PACKETS=true to save logs infos.
func SaveGraph(path string, withSend, withRcv bool) {
	fabric.Logger.Info().Msgf("Saving graph file to %s", path)
	f, _ := os.Create(path)
	GenerateGraphviz(f, withSend, withRcv, traffics...)
}

// traffic is used to keep track of packets traffic in a server
type traffic struct {
	name     string
	items    []item
	log      bool
	printLog bool
}

func newTraffic(name string) *traffic {
	log := false
	printLog := false

	flag := os.Getenv("MINO_LOG_PACKETS")
	if flag == "true" {
		log = true
	}

	flag = os.Getenv("MINO_PRINT_PACKETS")
	if flag == "true" {
		printLog = true
	}

	traffic := &traffic{
		name:     name,
		items:    make([]item, 0),
		log:      log,
		printLog: printLog,
	}

	traffics = append(traffics, traffic)

	return traffic
}

func (t *traffic) logSend(from, to mino.Address, msg *Envelope, context string) {
	t.addItem("send", from, to, msg, context)
}

func (t *traffic) logRcv(from, to mino.Address, msg *Envelope, context string) {
	t.addItem("received", from, to, msg, context)
}

func (t traffic) Display(out io.Writer) {
	fmt.Fprint(out, "- traffic:\n")
	var buf bytes.Buffer
	for _, item := range t.items {
		item.Display(&buf)
	}
	fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "-$1"))
}

func (t *traffic) addItem(typeStr string, from, to mino.Address, msg *Envelope,
	context string) {

	if !t.log {
		return
	}

	counter := sendCounter
	if typeStr == "received" {
		counter = recvCounter
	}

	newItem := item{
		typeStr:       typeStr,
		from:          from,
		to:            to,
		msg:           msg,
		context:       context,
		globalCounter: globalCounter.IncrementAndGet(),
		typeCounter:   counter.IncrementAndGet(),
	}

	if t.printLog {
		fmt.Fprintf(os.Stdout, "\n> %s", t.name)
		newItem.Display(os.Stdout)
	}

	t.items = append(t.items, newItem)
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
	fmt.Fprintf(out, "-- from: %s\n", p.from)
	fmt.Fprintf(out, "-- to: %s\n", p.to)
	fmt.Fprintf(out, "-- msg: (type %T) %s\n", p.msg, p.msg)
	fmt.Fprintf(out, "--- From: %s\n", p.msg.From)
	fmt.Fprintf(out, "--- PhysicalFrom: %s\n", p.msg.PhysicalFrom)
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
			err := ptypes.UnmarshalAny(item.msg.Message, &da)
			if err == nil {
				msgType = fmt.Sprintf("%T", da.Message)
			}

			color := "#4AB2FF"

			if item.typeStr == "received" {
				color = "#A8A8A8"
			}

			toStr := ""
			for _, to := range item.msg.To {
				toStr += fmt.Sprintf("to %s<br/>", to)
			}

			msgStr := fmt.Sprintf("<font point-size='10' color='#9C9C9C'>from %s<br/>%s%s</font>",
				item.msg.From, toStr, msgType)

			fmt.Fprintf(out, "\"%s\" -> \"%s\" "+
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
