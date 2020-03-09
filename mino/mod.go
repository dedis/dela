// Package mino provides an abstraction for an application layer. It offers a
// Minimalistic Overlay Network (MINO) to communicate between participants of
// a distributed system.
package mino

import (
	"context"
	"encoding"
	"errors"

	"github.com/golang/protobuf/proto"
)

// Address is a representation of a node's address.
type Address interface {
	encoding.TextMarshaler

	String() string
}

// AddressIterator is an iterator over the list of addresses of a membership.
type AddressIterator interface {
	Next() bool
	Get() Address
}

// Membership is an interface to represent a set of nodes participating in a
// message passing protocol.
type Membership interface {
	AddressIterator() AddressIterator
	Len() int
}

// Sender is an interface to provide primitives to send messages to recipients.
type Sender interface {
	Send(msg proto.Message, addrs ...Address) error
}

// Receiver is an interface to provide primitives to receive messages from
// recipients.
type Receiver interface {
	Recv(context.Context) (Address, proto.Message, error)
}

// RPC is a representation of a remote procedure call that can call a single
// distant procedure or multiple.
type RPC interface {
	// Call is a basic request to one or multiple distant peers.
	Call(req proto.Message, memship Membership) (<-chan proto.Message, <-chan error)

	// Stream is a persistent request that will be closed only when the
	// orchestrator is done or an error occured.
	Stream(ctx context.Context, memship Membership) (in Sender, out Receiver)
}

// Handler is the interface to implement to create a public endpoint.
type Handler interface {
	// Process handles a single request by producing the response according to
	// the request message.
	Process(req proto.Message) (resp proto.Message, err error)

	// Combine gives a chance to reduce the network load by combining multiple
	// messages for a collect call on the intermediate nodes.
	Combine(req []proto.Message) (resp []proto.Message, err error)

	// Stream is a handler for a stream request. It will open a stream with the
	// participants.
	Stream(in Sender, out Receiver) error
}

// UnsupportedHandler implements the Handler interface with default behaviour so
// that an implementation can focus on its needs.
type UnsupportedHandler struct{}

// Process is the default implementation for a handler. It will return an error.
func (h UnsupportedHandler) Process(req proto.Message) (proto.Message, error) {
	return nil, errors.New("rpc is not supported")
}

// Combine returns the messages without combining them.
func (h UnsupportedHandler) Combine(req []proto.Message) ([]proto.Message, error) {
	return req, nil
}

// Stream is the default implementation for a handler. It will return an error.
func (h UnsupportedHandler) Stream(in Sender, out Receiver) error {
	return errors.New("stream is not supported")
}

// AddressFactory is the factory to decode addresses.
type AddressFactory interface {
	FromText(text []byte) Address
}

// Mino is a representation of a overlay network that allows the creation
// of namespaces for internal protocols and associate handlers to it.
type Mino interface {
	GetAddressFactory() AddressFactory

	// Address returns the address that other participants should use to contact
	// this instance.
	GetAddress() Address

	// MakeNamespace returns an instance restricted to the namespace.
	MakeNamespace(namespace string) (Mino, error)

	// MakeRPC creates an RPC that can send to and receive from a uniq URI which
	// is computed with URI = (namespace || name)
	// The namespace is known by the minion instance.
	MakeRPC(name string, h Handler) (RPC, error)
}
