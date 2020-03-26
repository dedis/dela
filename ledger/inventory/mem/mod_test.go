package mem

import (
	"bytes"
	"hash"
	"io"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestInMemoryInventory(t *testing.T) {
	inv := NewInventory()
	inv.pages = []inMemoryPage{{}}

	page, err := inv.GetPage(0)
	require.NoError(t, err)
	require.IsType(t, inMemoryPage{}, page)

	_, err = inv.GetPage(2)
	require.EqualError(t, err, "invalid page at index 2")
}

func TestInMemoryInventory_Stage(t *testing.T) {
	inv := NewInventory()
	inv.pages = nil

	page, err := inv.Stage(func(page inventory.WritablePage) error {
		require.NoError(t, page.Write(inMemoryInstance{key: []byte{1}}))
		require.NoError(t, page.Write(inMemoryInstance{key: []byte{2}}))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, uint64(0), page.GetIndex())
	require.Len(t, inv.stagingPages, 1)
	require.Len(t, inv.pages, 0)

	inv.pages = append(inv.pages, inv.stagingPages[page.(inMemoryPage).root])
	inv.stagingPages = make(map[Digest]inMemoryPage)
	page, err = inv.Stage(func(page inventory.WritablePage) error {
		instance, err := page.Read([]byte{1})
		require.NoError(t, err)
		require.Equal(t, []byte{1}, instance.GetKey())
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), page.GetIndex())
	require.Len(t, inv.stagingPages, 1)
	require.Len(t, inv.pages, 1)

	_, err = inv.Stage(func(inventory.WritablePage) error {
		return xerrors.New("oops")
	})
	require.EqualError(t, err, "couldn't fill new page: oops")

	inv.hashFactory = badHashFactory{}
	_, err = inv.Stage(func(inventory.WritablePage) error {
		return nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't compute page hash: ")
}

func TestInMemoryInventory_Commit(t *testing.T) {
	digest := Digest{123}
	inv := NewInventory()
	inv.stagingPages[digest] = inMemoryPage{}

	err := inv.Commit(digest[:])
	require.NoError(t, err)

	err = inv.Commit([]byte{1, 2, 3, 4})
	require.EqualError(t, err, "couldn't find page with root '0x01020304'")
}

func TestInstance_GetKey(t *testing.T) {
	instance := inMemoryInstance{}
	require.Nil(t, instance.GetKey())

	instance.key = []byte{0xab}
	require.Equal(t, []byte{0xab}, instance.GetKey())
}

func TestInstance_GetValue(t *testing.T) {
	instance := inMemoryInstance{}
	require.Nil(t, instance.GetValue())

	instance.value = &wrappers.StringValue{Value: "abc"}
	require.Equal(t, instance.value, instance.GetValue())
}

func TestInstance_WriteTo(t *testing.T) {
	instance := inMemoryInstance{}

	buffer := new(bytes.Buffer)
	n, err := instance.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)
	require.Len(t, buffer.Bytes(), 0)

	instance.key = []byte{0xab}
	instance.value = &wrappers.BoolValue{Value: true}
	n, err = instance.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, int64(3), n)

	_, err = instance.WriteTo(&badWriter{delay: 0})
	require.EqualError(t, err, "couldn't write the key: oops")

	_, err = instance.WriteTo(&badWriter{delay: 1})
	require.EqualError(t, err, "couldn't write the value: oops")
}

func TestPage_GetIndex(t *testing.T) {
	f := func(index uint64) bool {
		page := inMemoryPage{index: index}
		return index == page.GetIndex()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPage_GetRoot(t *testing.T) {
	f := func(root Digest) bool {
		page := inMemoryPage{root: root}
		return bytes.Equal(root[:], page.GetRoot())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPage_Read(t *testing.T) {
	page := inMemoryPage{
		instances: map[Digest]inMemoryInstance{
			Digest{1}: {key: []byte{1}},
			Digest{2}: {key: []byte{2}},
		},
	}

	instance, err := page.Read([]byte{1})
	require.NoError(t, err)
	require.Equal(t, []byte{1}, instance.GetKey())

	badKey := [digestLength + 1]byte{}
	_, err = page.Read(badKey[:])
	require.EqualError(t, err, "key length (33) is higher than 32")

	_, err = page.Read([]byte{3})
	require.EqualError(t, err, "instance with key '0x03' not found")
}

func TestPage_Write(t *testing.T) {
	page := inMemoryPage{
		instances: make(map[Digest]inMemoryInstance),
	}

	err := page.Write(inMemoryInstance{key: []byte{1}})
	require.NoError(t, err)
	require.Len(t, page.instances, 1)

	err = page.Write(inMemoryInstance{key: []byte{2}})
	require.NoError(t, err)
	require.Len(t, page.instances, 2)

	err = page.Write(inMemoryInstance{key: []byte{1}})
	require.NoError(t, err)
	require.Len(t, page.instances, 2)

	badKey := [digestLength + 1]byte{}
	err = page.Write(inMemoryInstance{key: badKey[:]})
	require.EqualError(t, err, "key length (33) is higher than 32")
}

// -----------------------------------------------------------------------------
// Utility functions

type badWriter struct {
	io.Writer
	delay int
}

func (w *badWriter) Write([]byte) (int, error) {
	if w.delay == 0 {
		return 0, xerrors.New("oops")
	}
	w.delay--
	return 0, nil
}

type badHash struct {
	hash.Hash
}

func (h badHash) Write([]byte) (int, error) {
	return 0, xerrors.New("oops")
}

type badHashFactory struct {
	crypto.HashFactory
}

func (f badHashFactory) New() hash.Hash {
	return badHash{}
}
