package minogrpc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

var eachLine = regexp.MustCompile(`(?m)^(.+)$`)

var counter = atomicCounter{}

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

	return &traffic{
		name:     name,
		items:    make([]item, 0),
		log:      log,
		printLog: printLog,
	}
}

func (t *traffic) logSend(to mino.Address, msg proto.Message, context string) {
	t.addItem("send", to, msg, context)
}

func (t *traffic) logRcv(from mino.Address, msg proto.Message, context string) {
	t.addItem("received", from, msg, context)
}

func (t *traffic) logRcvRelay(from mino.Address, msg proto.Message, context string) {
	t.addItem("received to relay", from, msg, context)
}

func (t traffic) Display(out io.Writer) {
	fmt.Fprint(out, "- traffic:\n")
	var buf bytes.Buffer
	for _, item := range t.items {
		item.Display(&buf)
	}
	fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "-$1"))
}

func (t *traffic) addItem(typeStr string, addr mino.Address, msg proto.Message,
	context string) {

	if !t.log {
		return
	}

	newItem := item{
		typeStr: typeStr, addr: addr, msg: msg,
		context: context, counter: counter.IncrementAndGet()}

	if t.printLog {
		fmt.Fprintf(os.Stdout, "\n> %s", t.name)
		newItem.Display(os.Stdout)
	}

	t.items = append(t.items, newItem)
}

type item struct {
	typeStr string
	addr    mino.Address
	msg     proto.Message
	context string
	counter int
}

func (p item) Display(out io.Writer) {
	fmt.Fprint(out, "- item:\n")
	fmt.Fprintf(out, "-- typeStr: %s\n", p.typeStr)
	fmt.Fprintf(out, "-- addr: %s\n", p.addr)
	fmt.Fprintf(out, "-- msg: (type %T) %s\n", p.msg, p.msg)
	overlayMsg, ok := p.msg.(*OverlayMsg)
	if ok {
		envelope := &Envelope{}
		err := encoding.NewProtoEncoder().UnmarshalAny(overlayMsg.Message, envelope)
		if err == nil {
			fmt.Fprintf(out, "--- %s\n", envelope)
		}
	}
	fmt.Fprintf(out, "-- context: %s\n", p.context)
}

// GenerateGraphviz creates a graphviz representation
func GenerateGraphviz(out io.Writer, traffics ...*traffic) {
	fmt.Fprintf(out, "digraph network_activity {\n")
	for _, traffic := range traffics {
		for _, item := range traffic.items {
			var msgStr string
			overlayMsg, okOverlay := item.msg.(*OverlayMsg)
			envelope, okEnvelope := item.msg.(*Envelope)
			if okOverlay {
				envelope := &Envelope{}
				err := encoding.NewProtoEncoder().UnmarshalAny(
					overlayMsg.Message, envelope)
				if err == nil {
					msgStr = fmt.Sprintf("Envelope\\nfrom=%s\\nto=%v\\nmsg=%T",
						envelope.From, envelope.To, envelope.Message)
				} else {
					msgStr = fmt.Sprintf("%T", overlayMsg.Message)
				}
			} else if okEnvelope {
				msgStr = fmt.Sprintf("Envelope\\nfrom=%s\\nto=%v\\nmsg=%T",
					envelope.From, envelope.To, envelope.Message)
			} else {
				msgStr = fmt.Sprintf("%T", item.msg)
			}
			if item.typeStr == "send" {
				fmt.Fprintf(out, "\"%s(%s)\" -> \"%s\" "+
					"[ label = \"%d: %s\" color=\"blue\" ];\n",
					traffic.name, item.context, item.addr, item.counter, msgStr)
			} else if item.typeStr == "received" {
				fmt.Fprintf(out, "\"%s\" -> \"%s(%s)\" "+
					"[ label = \"%d: %s\" color=\"grey\" ];\n",
					item.addr, traffic.name, item.context, item.counter, msgStr)
			} else if item.typeStr == "received to relay" {
				fmt.Fprintf(out, "\"%s\" -> \"%s(%s)\" "+
					"[ label = \"%d: %s\" color=\"yellow\" ];\n",
					item.addr, traffic.name, item.context, item.counter, msgStr)
			}
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
