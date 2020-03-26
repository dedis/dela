package mem

import (
	"bytes"

	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

// InMemoryInventory is an implementation of the inventory interface by using a
// memory storage which means that it will not persist.
type InMemoryInventory struct {
	snapshots []snapshot
	queued    []snapshot
}

// NewInventory returns a new empty instance of the inventory.
func NewInventory() *InMemoryInventory {
	return &InMemoryInventory{
		snapshots: []snapshot{{}},
		queued:    []snapshot{},
	}
}

// GetSnapshot returns the snapshot for the version if it exists, otherwise an
// error.
func (inv *InMemoryInventory) GetSnapshot(version uint64) (inventory.Snapshot, error) {
	index := int(version)
	if index >= len(inv.snapshots) {
		return snapshot{}, xerrors.Errorf("invalid version %d", version)
	}

	return inv.snapshots[index], nil
}

// Stage starts a new version. It returns the new snapshot that is not yet
// committed to the available versions.
func (inv *InMemoryInventory) Stage(f func(inventory.WritableSnapshot) error) (inventory.Snapshot, error) {
	newSnap := snapshot{
		version:   uint64(len(inv.snapshots)),
		instances: make([]instance, 0),
	}

	err := f(newSnap)
	if err != nil {
		return newSnap, xerrors.Errorf("couldn't fill new snapshot: %v", err)
	}

	newSnap.root = []byte("deadbeef")

	inv.queued = append(inv.queued, newSnap)

	return newSnap, nil
}

// Commit stores the snapshot with the given root permanently to the list of
// available versions.
func (inv *InMemoryInventory) Commit(root []byte) error {
	for _, snap := range inv.queued {
		if bytes.Equal(snap.GetRoot(), root) {
			inv.snapshots = append(inv.snapshots, snap)
			inv.queued = []snapshot{}
			return nil
		}
	}

	return xerrors.New("not found")
}

type instance struct {
	key   []byte
	value interface{}
}

func (i instance) GetKey() []byte {
	return i.key
}

func (i instance) GetValue() interface{} {
	return i.value
}

type snapshot struct {
	version   uint64
	root      []byte
	instances []instance
}

func (snap snapshot) GetVersion() uint64 {
	return snap.version
}

func (snap snapshot) GetRoot() []byte {
	return snap.root
}

func (snap snapshot) Read(key []byte) (inventory.Instance, error) {
	for _, instance := range snap.instances {
		if bytes.Equal(instance.key, key) {
			return instance, nil
		}
	}

	return nil, xerrors.New("not found")
}

func (snap snapshot) Write(new inventory.Instance) error {
	for i, inst := range snap.instances {
		if bytes.Equal(inst.key, new.GetKey()) {
			snap.instances[i] = new.(instance)
			return nil
		}
	}

	snap.instances = append(snap.instances, new.(instance))

	return nil
}
