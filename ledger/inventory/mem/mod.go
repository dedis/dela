package mem

import (
	"fmt"
	"hash"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

const (
	digestLength = 32
)

// Digest is the type of ID for a page.
type Digest [digestLength]byte

func (d Digest) String() string {
	return fmt.Sprintf("%#x", d[:4])
}

// InMemoryInventory is an implementation of the inventory interface by using a
// memory storage which means that it will not persist.
//
// - implements inventory.Inventory
type InMemoryInventory struct {
	hashFactory  crypto.HashFactory
	pages        []inMemoryPage
	stagingPages map[Digest]inMemoryPage
}

// NewInventory returns a new empty instance of the inventory.
func NewInventory() *InMemoryInventory {
	return &InMemoryInventory{
		hashFactory:  crypto.NewSha256Factory(),
		pages:        []inMemoryPage{},
		stagingPages: make(map[Digest]inMemoryPage),
	}
}

// GetPage implements inventory.Inventory. It returns the snapshot for the
// version if it exists, otherwise an error.
func (inv *InMemoryInventory) GetPage(index uint64) (inventory.Page, error) {
	i := int(index)
	if i >= len(inv.pages) {
		return inMemoryPage{}, xerrors.Errorf("invalid page (%d >= %d)", i, len(inv.pages))
	}

	return inv.pages[i], nil
}

// GetStagingPage implements inventory.Inventory. It returns the staging page
// that matches the root if any, otherwise nil.
func (inv *InMemoryInventory) GetStagingPage(root []byte) inventory.Page {
	digest := Digest{}
	copy(digest[:], root)

	page, ok := inv.stagingPages[digest]
	if !ok {
		return nil
	}

	return page
}

// Stage implements inventory.Inventory. It starts a new version. It returns the
// new snapshot that is not yet committed to the available versions.
func (inv *InMemoryInventory) Stage(f func(inventory.WritablePage) error) (inventory.Page, error) {
	var page inMemoryPage
	if len(inv.pages) > 0 {
		// Clone the previous page of the inventory so that previous instances
		// are carried over.
		page = inv.pages[len(inv.pages)-1].clone()
		page.index++
	} else {
		page.entries = make(map[Digest]inMemoryEntry)
	}

	err := f(page)
	if err != nil {
		return page, xerrors.Errorf("couldn't fill new page: %v", err)
	}

	page.footprint, err = page.computeHash(inv.hashFactory)
	if err != nil {
		return page, xerrors.Errorf("couldn't compute page hash: %v", err)
	}

	inv.stagingPages[page.footprint] = page

	return page, nil
}

// Commit stores the page with the given footprint permanently to the list of
// available versions.
func (inv *InMemoryInventory) Commit(footprint []byte) error {
	digest := Digest{}
	copy(digest[:], footprint)

	page, ok := inv.stagingPages[digest]
	if !ok {
		return xerrors.Errorf("couldn't find page with footprint '%v'", digest)
	}

	inv.pages = append(inv.pages, page)
	inv.stagingPages = make(map[Digest]inMemoryPage)

	return nil
}

// inMemoryEntry is an instance stored in an in-memory inventory.
type inMemoryEntry struct {
	value proto.Message
}

func (i inMemoryEntry) hash(h hash.Hash) error {
	// The JSON format is used to insure a deterministic hash.
	m := &jsonpb.Marshaler{OrigName: true}
	err := m.Marshal(h, i.value)
	if err != nil {
		return err
	}

	return nil
}

// inMemoryPage is an implementation of the Page interface for an inventory. It
// holds in memory the instances that have been created up to that index.
//
// - implements inventory.Page
// - implements inventory.WritablePage
type inMemoryPage struct {
	index     uint64
	footprint Digest
	entries   map[Digest]inMemoryEntry
}

// GetIndex implements inventory.Page. It returns the index of the page from the
// beginning of the inventory.
func (page inMemoryPage) GetIndex() uint64 {
	return page.index
}

// GetFootprint implements inventory.Page. It returns the integrity footprint of the
// page.
func (page inMemoryPage) GetFootprint() []byte {
	return page.footprint[:]
}

// Read implements inventory.Page. It returns the instance associated with the
// key if it exists, otherwise an error.
func (page inMemoryPage) Read(key []byte) (proto.Message, error) {
	if len(key) > digestLength {
		return nil, xerrors.Errorf("key length (%d) is higher than %d",
			len(key), digestLength)
	}

	digest := Digest{}
	copy(digest[:], key)

	entry, ok := page.entries[digest]
	if !ok {
		return nil, xerrors.Errorf("instance with key '%#x' not found", key)
	}

	return entry.value, nil
}

// Write implements inventory.WritablePage. It updates the state of the page by
// adding or updating the instance.
func (page inMemoryPage) Write(key []byte, value proto.Message) error {
	if len(key) > digestLength {
		return xerrors.Errorf("key length (%d) is higher than %d", len(key), digestLength)
	}

	digest := Digest{}
	copy(digest[:], key)

	page.entries[digest] = inMemoryEntry{value: value}

	return nil
}

func (page inMemoryPage) clone() inMemoryPage {
	clone := inMemoryPage{
		index:   page.index,
		entries: make(map[Digest]inMemoryEntry),
	}

	for k, v := range page.entries {
		clone.entries[k] = v
	}

	return clone
}

func (page inMemoryPage) computeHash(factory crypto.HashFactory) (Digest, error) {
	h := factory.New()

	for _, entry := range page.entries {
		err := entry.hash(h)
		if err != nil {
			return Digest{}, err
		}
	}

	digest := Digest{}
	copy(digest[:], h.Sum(nil))
	return digest, nil
}
