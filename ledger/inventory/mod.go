package inventory

import "github.com/golang/protobuf/proto"

// Instance is the smallest unit of storage of a ledger.
type Instance interface {
	GetKey() []byte
	GetValue() proto.Message
}

// Page represents the state of the inventory at the given point in time.
type Page interface {
	GetIndex() uint64
	GetRoot() []byte
	Read(key []byte) (Instance, error)
}

// WritablePage is an upgradable snapshot that can be committed later on.
type WritablePage interface {
	Page

	Write(instance Instance) error
}

// Inventory is an abstraction of the state of the ledger at different point in
// time. It can be modified using a two-phase commit procedure.
type Inventory interface {
	// GetPage returns a snapshot of the state of the inventory at the block
	// with the given index.
	GetPage(index uint64) (Page, error)

	// Stage starts a new version of the inventory and temporarily stores it
	// until it is committed or another staging version is.
	Stage(func(WritablePage) error) (Page, error)

	// Commit commits the new version with the identifier.
	Commit([]byte) error
}
