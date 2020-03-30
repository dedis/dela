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

func TestInMemoryInventory_GetPage(t *testing.T) {
	inv := NewInventory()
	inv.pages = []inMemoryPage{{}}

	page, err := inv.GetPage(0)
	require.NoError(t, err)
	require.IsType(t, inMemoryPage{}, page)

	_, err = inv.GetPage(2)
	require.EqualError(t, err, "invalid page (2 >= 1)")
}

func TestInMemoryInventory_GetStagingPage(t *testing.T) {
	inv := NewInventory()
	inv.stagingPages[Digest{1}] = inMemoryPage{}

	require.Nil(t, inv.GetStagingPage([]byte{2}))
	require.NotNil(t, inv.GetStagingPage([]byte{1}))
}

func TestInMemoryInventory_Stage(t *testing.T) {
	inv := NewInventory()
	inv.pages = nil

	value := &wrappers.BoolValue{Value: true}

	page, err := inv.Stage(func(page inventory.WritablePage) error {
		require.NoError(t, page.Write([]byte{1}, value))
		require.NoError(t, page.Write([]byte{2}, value))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, uint64(0), page.GetIndex())
	require.Len(t, inv.stagingPages, 1)
	require.Len(t, inv.pages, 0)

	inv.pages = append(inv.pages, inv.stagingPages[page.(inMemoryPage).footprint])
	inv.stagingPages = make(map[Digest]inMemoryPage)
	page, err = inv.Stage(func(page inventory.WritablePage) error {
		value, err := page.Read([]byte{1})
		require.NoError(t, err)
		require.True(t, value.(*wrappers.BoolValue).Value)
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
	require.EqualError(t, err, "couldn't find page with footprint '0x01020304'")
}

func TestPage_GetIndex(t *testing.T) {
	f := func(index uint64) bool {
		page := inMemoryPage{index: index}
		return index == page.GetIndex()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPage_GetFootprint(t *testing.T) {
	f := func(footprint Digest) bool {
		page := inMemoryPage{footprint: footprint}
		return bytes.Equal(footprint[:], page.GetFootprint())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPage_Read(t *testing.T) {
	page := inMemoryPage{
		entries: map[Digest]inMemoryEntry{
			Digest{1}: {value: &wrappers.StringValue{Value: "1"}},
			Digest{2}: {value: &wrappers.StringValue{Value: "2"}},
		},
	}

	value, err := page.Read([]byte{1})
	require.NoError(t, err)
	require.Equal(t, "1", value.(*wrappers.StringValue).Value)

	badKey := [digestLength + 1]byte{}
	_, err = page.Read(badKey[:])
	require.EqualError(t, err, "key length (33) is higher than 32")

	_, err = page.Read([]byte{3})
	require.EqualError(t, err, "instance with key '0x03' not found")
}

func TestPage_Write(t *testing.T) {
	page := inMemoryPage{
		entries: make(map[Digest]inMemoryEntry),
	}

	value := &wrappers.BoolValue{Value: true}

	err := page.Write([]byte{1}, value)
	require.NoError(t, err)
	require.Len(t, page.entries, 1)

	err = page.Write([]byte{2}, value)
	require.NoError(t, err)
	require.Len(t, page.entries, 2)

	err = page.Write([]byte{1}, value)
	require.NoError(t, err)
	require.Len(t, page.entries, 2)

	badKey := [digestLength + 1]byte{}
	err = page.Write(badKey[:], value)
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
