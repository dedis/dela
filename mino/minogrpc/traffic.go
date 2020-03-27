package minogrpc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/mino"
)

var eachLine = regexp.MustCompile(`(?m)^(.+)$`)

// traffic is used to keep track of packets traffic in a server
type traffic struct {
	items []item
}

func (h traffic) String() string {
	out := new(strings.Builder)
	out.WriteString("- traffic:\n")
	for _, item := range h.items {
		out.WriteString(eachLine.ReplaceAllString(item.String(), "-$1"))
	}
	return out.String()
}

func (h *traffic) addItem(typeStr string, addr mino.Address, msg proto.Message, context string) {
	h.items = append(h.items, item{typeStr: typeStr, addr: addr, msg: msg, context: context})
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
