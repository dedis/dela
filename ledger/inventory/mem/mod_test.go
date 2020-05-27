package mem

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestInMemoryInventory_GetPage(t *testing.T) {
	inv := NewInventory()
	inv.pages = []*inMemoryPage{{}}

	page, err := inv.GetPage(0)
	require.NoError(t, err)
	require.IsType(t, (*inMemoryPage)(nil), page)

	_, err = inv.GetPage(2)
	require.EqualError(t, err, "invalid page (2 >= 1)")
}

func TestInMemoryInventory_GetStagingPage(t *testing.T) {
	inv := NewInventory()
	inv.stagingPages[Digest{1}] = &inMemoryPage{}

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

	inv.pages = append(inv.pages, inv.stagingPages[page.(*inMemoryPage).fingerprint])
	inv.stagingPages = make(map[Digest]*inMemoryPage)
	page, err = inv.Stage(func(page inventory.WritablePage) error {
		page.Defer(func([]byte) {})
		value, err := page.Read([]byte{1})
		require.NoError(t, err)
		require.True(t, value.(*wrappers.BoolValue).Value)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), page.GetIndex())
	require.Len(t, inv.stagingPages, 1)
	require.Len(t, inv.pages, 1)

	// Check stability of the hash of the page.
	mempage := page.(*inMemoryPage)
	for i := 0; i < 10; i++ {
		require.NoError(t, inv.computeHash(mempage))
		_, ok := inv.stagingPages[mempage.fingerprint]
		require.True(t, ok)
	}

	_, err = inv.Stage(func(inventory.WritablePage) error {
		return xerrors.New("oops")
	})
	require.EqualError(t, err, "couldn't fill new page: oops")

	inv.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = inv.Stage(func(inventory.WritablePage) error { return nil })
	require.EqualError(t, err,
		"couldn't compute page hash: couldn't write index: fake error")

	inv.hashFactory = fake.NewHashFactory(fake.NewBadHashWithDelay(1))
	_, err = inv.Stage(func(inventory.WritablePage) error { return nil })
	require.EqualError(t, err,
		"couldn't compute page hash: couldn't write key: fake error")

	inv.hashFactory = fake.NewHashFactory(&fake.Hash{})
	inv.encoder = fake.BadMarshalStableEncoder{}
	_, err = inv.Stage(func(inventory.WritablePage) error { return nil })
	require.EqualError(t, err,
		"couldn't compute page hash: couldn't marshal entry: fake error")
}

func TestInMemoryInventory_Commit(t *testing.T) {
	digest := Digest{123}
	inv := NewInventory()
	inv.stagingPages[digest] = &inMemoryPage{}

	err := inv.Commit(digest[:])
	require.NoError(t, err)

	err = inv.Commit([]byte{1, 2, 3, 4})
	require.EqualError(t, err, "couldn't find page with fingerprint '0x01020304'")
}

func TestPage_GetIndex(t *testing.T) {
	f := func(index uint64) bool {
		page := inMemoryPage{index: index}
		return index == page.GetIndex()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPage_GetFingerprint(t *testing.T) {
	f := func(fingerprint Digest) bool {
		page := inMemoryPage{fingerprint: fingerprint}
		return bytes.Equal(fingerprint[:], page.GetFingerprint())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPage_Read(t *testing.T) {
	page := inMemoryPage{
		entries: map[Digest]proto.Message{
			{1}: &wrappers.StringValue{Value: "1"},
			{2}: &wrappers.StringValue{Value: "2"},
		},
	}

	value, err := page.Read([]byte{1})
	require.NoError(t, err)
	require.Equal(t, "1", value.(*wrappers.StringValue).Value)

	value, err = page.Read([]byte{3})
	require.NoError(t, err)
	require.Nil(t, value)

	badKey := [digestLength + 1]byte{}
	_, err = page.Read(badKey[:])
	require.EqualError(t, err, "key length (33) is higher than 32")
}

func TestPage_Write(t *testing.T) {
	page := inMemoryPage{
		entries: make(map[Digest]proto.Message),
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
