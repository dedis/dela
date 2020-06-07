package inventory

import "github.com/golang/protobuf/proto"

// Page represents the state of the inventory at some point in time.
type Page interface {
	// GetIndex returns the index of the page since the beginning of the
	// inventory.
	GetIndex() uint64

	// GetFingerprint returns the fingerprint of the page. It can be used to
	// verify the integrity.
	GetFingerprint() []byte

	// Read returns the value stored at the given key. If the key does not
	// exist, it should return a nil value without error.
	Read(key []byte) (proto.Message, error)
}

// WritablePage is an upgradable snapshot that can be committed later on.
type WritablePage interface {
	Page

	// Write stores the value with the key as an identifier.
	Write(key []byte, value proto.Message) error
}

// Inventory is an abstraction of the state of the ledger at different point in
// time. It can be modified using a two-phase commit procedure.
type Inventory interface {
	// Len returns the length of the inventory.
	Len() uint64

	// GetPage returns a snapshot of the state of the inventory at the block
	// with the given index.
	GetPage(index uint64) (Page, error)

	// GetStagingPage returns the staging page that matches the root if any,
	// otherwise nil.
	GetStagingPage(root []byte) Page

	// Stage starts a new version of the inventory and temporarily stores it
	// until it is committed or another staging version is.
	Stage(func(WritablePage) error) (Page, error)

	// Commit commits the new version with the identifier.
	Commit(fingerprint []byte) error

	// Range iterates over the staging pages until the end of the function
	// returns false.
	Range(func(Page) bool)
}
