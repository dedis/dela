package inventory

// Instance is the smallest unit of storage of a ledger.
type Instance interface {
	GetKey() []byte
	GetValue() interface{}
}

// Snapshot represents the state of the inventory at the given point in time.
type Snapshot interface {
	GetVersion() uint64
	GetRoot() []byte
	Read(key []byte) (Instance, error)
}

// WritableSnapshot is an upgradable snapshot that can be committed later on.
type WritableSnapshot interface {
	Snapshot

	Write(instance Instance) error
}

// Inventory is an abstraction of the state of the ledger at different point in
// time. It can be modified using a two-phase commit procedure.
type Inventory interface {
	// GetSnapshot returns a snapshot of the state of the inventory at the block
	// with the given index.
	GetSnapshot(version uint64) (Snapshot, error)

	// Stage starts a new version of the inventory and temporarily stores it
	// until it is committed or another staging version is.
	Stage(func(WritableSnapshot) error) (Snapshot, error)

	// Commit commits the new version with the identifier.
	Commit([]byte) error
}
