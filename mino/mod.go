// Package mino provides an abstraction for an application layer. It offers a
// Minimalistic Overlay Network (MINO) to communicate between participants of
// a distributed system.
package mino

import (
	"context"
	"encoding"
	"errors"

	"go.dedis.ch/dela/serde"
)

// Address is a representation of a node's address.
type Address interface {
	encoding.TextMarshaler

	Equal(other Address) bool

	String() string
}

// AddressIterator is an iterator over the list of addresses of a membership.
type AddressIterator interface {
	// Seek moves the iterator to a specific index.
	Seek(int)

	// HasNext returns true if a address is available, false if the iterator is
	// exhausted.
	HasNext() bool

	// GetNext returns the next address in case HasNext returns true, otherwise
	// no assumption can be done.
	GetNext() Address
}

// Players is an interface to represent a set of nodes participating in a
// message passing protocol.
type Players interface {
	// Take should a subset of the players according to the filters.
	Take(...FilterUpdater) Players

	// AddressIterator returns an iterator that prevents changes of the
	// underlying array and save memory by iterating over the same array.
	AddressIterator() AddressIterator

	// Len returns the length of the set of players.
	Len() int
}

// Sender is an interface to provide primitives to send messages to recipients.
type Sender interface {
	// Send sends the message to all the addresses. It returns a channel that
	// will be populated with errors coming from the network layer if the
	// message cannot be sent. The channel must be closed after the message has
	// been/failed to be sent.
	Send(msg serde.Message, addrs ...Address) <-chan error
}

// Receiver is an interface to provide primitives to receive messages from
// recipients.
type Receiver interface {
	Recv(context.Context) (Address, serde.Message, error)
}

// Request is a wrapper around the context of a message received from a player
// and that needs to be processed by the node. It provides some useful
// information about the network layer.
type Request struct {
	// Address is the address of the sender of the request.
	Address Address

	// Message is the message of the request.
	Message serde.Message
}

// Response is a interface that Mino implementations should comply with. The
// response can either contain a message, or an error if something wrong
// happened.
type Response interface {
	// GetFrom returns the address of the source of the reply.
	GetFrom() Address

	// GetMessageOrError returns the message, or an error if something wrong
	// happened.
	GetMessageOrError() (serde.Message, error)
}

// RPC is a representation of a remote procedure call that can call a single
// distant procedure or multiple.
type RPC interface {
	// Call is a basic request to one or multiple distant peers. It directly
	// contacts all the players and thus expect a reasonable number of peers.
	//
	// The response channel must be closed after every request ended in a
	// result, either a reply or an error.
	Call(ctx context.Context, req serde.Message, players Players) (<-chan Response, error)

	// Stream is a persistent request that will be closed only when the
	// orchestrator is done or an error occured.
	Stream(ctx context.Context, players Players) (Sender, Receiver, error)
}

// Handler is the interface to implement to create a public endpoint.
type Handler interface {
	// Process handles a single request by producing the response according to
	// the request message.
	Process(req Request) (resp serde.Message, err error)

	// Stream is a handler for a stream request. It will open a stream with the
	// participants.
	Stream(out Sender, in Receiver) error
}

// UnsupportedHandler implements the Handler interface with default behaviour so
// that an implementation can focus on its needs.
type UnsupportedHandler struct{}

// Process is the default implementation for a handler. It will return an error.
func (h UnsupportedHandler) Process(req Request) (serde.Message, error) {
	return nil, errors.New("rpc is not supported")
}

// Stream is the default implementation for a handler. It will return an error.
func (h UnsupportedHandler) Stream(in Sender, out Receiver) error {
	return errors.New("stream is not supported")
}

// AddressFactory is the factory to decode addresses.
type AddressFactory interface {
	serde.Factory

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

	// MakeRPC creates an RPC that can send to and receive from a unique URI
	// which is computed with URI = (namespace || name). The namespace is known
	// by the mino instance.
	MakeRPC(name string, h Handler, f serde.Factory) (RPC, error)
}
