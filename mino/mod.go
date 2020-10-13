// Package mino defines a minimalist network overlay to communicate between a
// set of participants.
//
// The overlay provides two primitives to send messages: Call that will directly
// contact the addresses and Stream that will build a network and distribute the
// load to forward the messages.
//
// Documentation Last Review: 07.10.2020
//
package mino

import (
	"context"
	"encoding"
	"errors"

	"go.dedis.ch/dela/serde"
)

// Mino is an abstraction of a overlay network. It provides primitives to send
// messages to a set of participants.
//
// It uses segments to separate the different RPCs of multiple services in a
// URI-like manner (i.e. /my/awesome/rpc has _my_ and _awesome_ segments).
type Mino interface {
	GetAddressFactory() AddressFactory

	// Address returns the address that other participants should use to contact
	// this instance.
	GetAddress() Address

	// WithSegment returns a new mino instance that will have its URI path
	// extended with the provided segment.
	WithSegment(segment string) Mino

	// CreateRPC creates an RPC that can send to and receive from a unique
	// URI which is computed with URI = (segment || segment || ... || name). If
	// the combination already exists, it will return an error.
	CreateRPC(name string, h Handler, f serde.Factory) (RPC, error)
}

// Address is a representation of a node's address.
type Address interface {
	encoding.TextMarshaler

	// Equal returns true when both addresses are similar.
	Equal(other Address) bool

	// String returns a string representation of the address.
	String() string
}

// AddressFactory is the factory to deserialize addresses.
type AddressFactory interface {
	serde.Factory

	// FromText returns the address of the text.
	FromText(text []byte) Address
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

// RPC is an abstraction of a distributed Remote Procedure Call.
type RPC interface {
	// Call is a basic request to one or multiple distant peers. It directly
	// contacts all the players and thus expects a reasonable number of peers.
	//
	// The response channel must be closed once the request ends in a result,
	// either a reply or an error.
	Call(ctx context.Context, req serde.Message, players Players) (<-chan Response, error)

	// Stream is a persistent request that will be closed only when the
	// orchestrator is done or an error has occured, or the context is done.
	Stream(ctx context.Context, players Players) (Sender, Receiver, error)
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

// Response represents the response of a distributed RPC. It provides the
// address of the original sender and the reply, or an error if something went
// wrong upfront.
type Response interface {
	// GetFrom returns the address of the source of the reply.
	GetFrom() Address

	// GetMessageOrError returns the message, or an error if something wrong
	// happened.
	GetMessageOrError() (serde.Message, error)
}

// Sender is an abstraction to send messages to a stream in the context of a
// distributed RPC.
type Sender interface {
	// Send sends the message to all the addresses. It returns a channel that
	// will be populated with errors coming from the network layer if the
	// message cannot be sent. The channel must be closed after the message has
	// been sent or failed to be sent.
	Send(msg serde.Message, addrs ...Address) <-chan error
}

// Receiver is an abstraction to receive messages from a stream in the context
// of a distributed RPC.
type Receiver interface {
	// Recv waits for a message to send received from the stream. It returns the
	// address of the original sender and the message, or a message if the
	// stream is closed or the context is done.
	Recv(context.Context) (Address, serde.Message, error)
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

// MustCreateRPC creates the RPC with the given name, handler and factory to the
// Mino instance and expects the operation to be successful, otherwise it will
// panic.
func MustCreateRPC(m Mino, name string, h Handler, f serde.Factory) RPC {
	rpc, err := m.CreateRPC(name, h, f)
	if err != nil {
		panic(err)
	}

	return rpc
}
