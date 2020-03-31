package minogrpc

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/mino"
)

var eachLine = regexp.MustCompile(`(?m)^(.+)$`)

// traffic is used to keep track of packets traffic in a server
type traffic struct {
	name  string
	items []item
	log   bool
}

func newTraffic(name string) *traffic {
	log := false

	flag := os.Getenv("MINO_LOG_PACKETS")
	if flag == "true" {
		log = true
	}

	return &traffic{
		name:  name,
		items: make([]item, 0),
		log:   log,
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

func (t traffic) String() string {
	out := new(strings.Builder)
	out.WriteString("- traffic:\n")
	for _, item := range t.items {
		out.WriteString(eachLine.ReplaceAllString(item.String(), "-$1"))
	}
	return out.String()
}

func (t *traffic) addItem(typeStr string, addr mino.Address, msg proto.Message, context string) {
	if !t.log {
		return
	}
	newItem := item{typeStr: typeStr, addr: addr, msg: msg, context: context}
	// fmt.Println("\n", t.name, newItem)
	t.items = append(t.items, newItem)
}

type item struct {
	typeStr string
	addr    mino.Address
	msg     proto.Message
	context string
}

func (p item) String() string {
	out := new(strings.Builder)
	out.WriteString("- item:\n")
	fmt.Fprintf(out, "-- typeStr: %s\n", p.typeStr)
	fmt.Fprintf(out, "-- addr: %s\n", p.addr)
	fmt.Fprintf(out, "-- msg: (type %T) %s\n", p.msg, p.msg)
	fmt.Fprintf(out, "-- context: %s\n", p.context)
	return out.String()
}