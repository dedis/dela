package mem

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
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

// DigestSlice is a sortable slice of digests. It allows to hash the inventory
// page in a deterministic manner.
type DigestSlice []Digest

func (p DigestSlice) Len() int {
	return len(p)
}

func (p DigestSlice) Less(i, j int) bool {
	return bytes.Compare(p[i][:], p[j][:]) < 0
}

func (p DigestSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// InMemoryInventory is an implementation of the inventory interface by using a
// memory storage which means that it will not persist.
//
// - implements inventory.Inventory
type InMemoryInventory struct {
	sync.Mutex
	encoder      encoding.ProtoMarshaler
	hashFactory  crypto.HashFactory
	pages        []*inMemoryPage
	stagingPages map[Digest]*inMemoryPage
}

// NewInventory returns a new empty instance of the inventory.
func NewInventory() *InMemoryInventory {
	return &InMemoryInventory{
		encoder:      encoding.NewProtoEncoder(),
		hashFactory:  crypto.NewSha256Factory(),
		pages:        []*inMemoryPage{},
		stagingPages: make(map[Digest]*inMemoryPage),
	}
}

// GetPage implements inventory.Inventory. It returns the snapshot for the
// version if it exists, otherwise an error.
func (inv *InMemoryInventory) GetPage(index uint64) (inventory.Page, error) {
	inv.Lock()
	defer inv.Unlock()

	i := int(index)
	if i >= len(inv.pages) {
		return nil, xerrors.Errorf("invalid page (%d >= %d)", i, len(inv.pages))
	}

	return inv.pages[i], nil
}

// GetStagingPage implements inventory.Inventory. It returns the staging page
// that matches the root if any, otherwise nil.
func (inv *InMemoryInventory) GetStagingPage(root []byte) inventory.Page {
	inv.Lock()
	defer inv.Unlock()

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
	var page *inMemoryPage
	inv.Lock()
	if len(inv.pages) > 0 {
		// Clone the previous page of the inventory so that previous instances
		// are carried over.
		page = inv.pages[len(inv.pages)-1].clone()
		page.index++
	} else {
		page = &inMemoryPage{
			entries: make(map[Digest]proto.Message),
		}
	}
	inv.Unlock()

	err := f(page)
	if err != nil {
		return page, xerrors.Errorf("couldn't fill new page: %v", err)
	}

	err = inv.computeHash(page)
	if err != nil {
		return page, xerrors.Errorf("couldn't compute page hash: %v", err)
	}

	inv.Lock()
	inv.stagingPages[page.fingerprint] = page
	inv.Unlock()

	for _, fn := range page.defers {
		fn(page.GetFingerprint())
	}

	return page, nil
}

func (inv *InMemoryInventory) computeHash(page *inMemoryPage) error {
	h := inv.hashFactory.New()

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, page.index)
	_, err := h.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write index: %v", err)
	}

	keys := make(DigestSlice, 0, len(page.entries))
	for key := range page.entries {
		keys = append(keys, key)
	}

	sort.Sort(keys)

	for _, key := range keys {
		_, err := h.Write(key[:])
		if err != nil {
			return xerrors.Errorf("couldn't write key: %v", err)
		}

		err = inv.encoder.MarshalStable(h, page.entries[key])
		if err != nil {
			return xerrors.Errorf("couldn't marshal entry: %v", err)
		}
	}

	page.fingerprint = Digest{}
	copy(page.fingerprint[:], h.Sum(nil))
	return nil
}

// Commit stores the page with the given fingerprint permanently to the list of
// available versions.
func (inv *InMemoryInventory) Commit(fingerprint []byte) error {
	inv.Lock()
	defer inv.Unlock()

	digest := Digest{}
	copy(digest[:], fingerprint)

	page, ok := inv.stagingPages[digest]
	if !ok {
		return xerrors.Errorf("couldn't find page with fingerprint '%v'", digest)
	}

	inv.pages = append(inv.pages, page)
	inv.stagingPages = make(map[Digest]*inMemoryPage)

	return nil
}

// inMemoryPage is an implementation of the Page interface for an inventory. It
// holds in memory the instances that have been created up to that index.
//
// - implements inventory.Page
// - implements inventory.WritablePage
type inMemoryPage struct {
	index       uint64
	fingerprint Digest
	defers      []func([]byte)
	entries     map[Digest]proto.Message
}

// GetIndex implements inventory.Page. It returns the index of the page from the
// beginning of the inventory.
func (page *inMemoryPage) GetIndex() uint64 {
	return page.index
}

// GetFingerprint implements inventory.Page. It returns the integrity
// fingerprint of the page.
func (page *inMemoryPage) GetFingerprint() []byte {
	return page.fingerprint[:]
}

// Read implements inventory.Page. It returns the instance associated with the
// key if it exists, otherwise an error.
func (page *inMemoryPage) Read(key []byte) (proto.Message, error) {
	if len(key) > digestLength {
		return nil, xerrors.Errorf("key length (%d) is higher than %d",
			len(key), digestLength)
	}

	digest := Digest{}
	copy(digest[:], key)

	return page.entries[digest], nil
}

// Write implements inventory.WritablePage. It updates the state of the page by
// adding or updating the instance.
func (page *inMemoryPage) Write(key []byte, value proto.Message) error {
	if len(key) > digestLength {
		return xerrors.Errorf("key length (%d) is higher than %d", len(key), digestLength)
	}

	digest := Digest{}
	copy(digest[:], key)

	page.entries[digest] = value

	return nil
}

func (page *inMemoryPage) Defer(fn func([]byte)) {
	page.defers = append(page.defers, fn)
}

func (page *inMemoryPage) clone() *inMemoryPage {
	clone := &inMemoryPage{
		index:   page.index,
		entries: make(map[Digest]proto.Message),
	}

	for k, v := range page.entries {
		clone.entries[k] = v
	}

	return clone
}
