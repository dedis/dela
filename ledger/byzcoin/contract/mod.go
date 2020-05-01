// Package contract is for static smart contracts.
package contract

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/transactions/basic"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Context is provided during a transaction execution.
type Context interface {
	basic.Context

	GetArc([]byte) (arc.AccessControl, error)

	Read([]byte) (*Instance, error)
}

// Contract is an interface that provides the primitives to execute a smart
// contract transaction and produce the resulting instance.
type Contract interface {
	// Spawn is called to create a new instance. It returns the initial value of
	// the new instance and its access rights control (arc) ID.
	Spawn(ctx SpawnContext) (proto.Message, []byte, error)

	// Invoke is called to update an existing instance.
	Invoke(ctx InvokeContext) (proto.Message, error)
}
