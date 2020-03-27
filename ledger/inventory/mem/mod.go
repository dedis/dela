package mem

import (
	"fmt"
	"io"

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
		page.instances = make(map[Digest]inMemoryInstance)
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

// inMemoryInstance is an instance stored in an in-memory inventory.
//
// - implements inventory.Instance
// - implements io.WriterTo
type inMemoryInstance struct {
	key   []byte
	value proto.Message
}

// GetKey implements inventory.Instance. It returns the key of the instance.
func (i inMemoryInstance) GetKey() []byte {
	return i.key
}

// GetValue implements inventory.Instance. It returns the value of the instance.
func (i inMemoryInstance) GetValue() proto.Message {
	return i.value
}

// WriteTo implements io.WriterTo. It writes the key and the value of the
// instance into the writer by using their bytes representation.
func (i inMemoryInstance) WriteTo(w io.Writer) (int64, error) {
	sum := int64(0)
	n, err := w.Write(i.key)
	sum += int64(n)
	if err != nil {
		return sum, xerrors.Errorf("couldn't write the key: %v", err)
	}

	if i.value == nil {
		return sum, nil
	}

	buffer, err := proto.Marshal(i.value)
	if err != nil {
		return sum, encoding.NewEncodingError("value", err)
	}

	n, err = w.Write(buffer)
	sum += int64(n)
	if err != nil {
		return sum, xerrors.Errorf("couldn't write the value: %v", err)
	}

	return sum, nil
}

// inMemoryPage is an implementation of the Page interface for an inventory. It
// holds in memory the instances that have been created up to that index.
//
// - implements inventory.Page
// - implements inventory.WritablePage
type inMemoryPage struct {
	index     uint64
	footprint Digest
	instances map[Digest]inMemoryInstance
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
func (page inMemoryPage) Read(key []byte) (inventory.Instance, error) {
	if len(key) > digestLength {
		return nil, xerrors.Errorf("key length (%d) is higher than %d",
			len(key), digestLength)
	}

	digest := Digest{}
	copy(digest[:], key)

	instance, ok := page.instances[digest]
	if !ok {
		return nil, xerrors.Errorf("instance with key '%#x' not found", key)
	}

	return instance, nil
}

// Write implements inventory.WritablePage. It updates the state of the page by
// adding or updating the instance.
func (page inMemoryPage) Write(instance inventory.Instance) error {
	if len(instance.GetKey()) > digestLength {
		return xerrors.Errorf("key length (%d) is higher than %d",
			len(instance.GetKey()), digestLength)
	}

	digest := Digest{}
	copy(digest[:], instance.GetKey())

	page.instances[digest] = instance.(inMemoryInstance)

	return nil
}

func (page inMemoryPage) clone() inMemoryPage {
	clone := inMemoryPage{
		index:     page.index,
		instances: make(map[Digest]inMemoryInstance),
	}

	for k, v := range page.instances {
		clone.instances[k] = v
	}

	return clone
}

func (page inMemoryPage) computeHash(factory crypto.HashFactory) (Digest, error) {
	h := factory.New()

	for _, instance := range page.instances {
		_, err := instance.WriteTo(h)
		if err != nil {
			return Digest{}, err
		}
	}

	digest := Digest{}
	copy(digest[:], h.Sum(nil))
	return digest, nil
}
