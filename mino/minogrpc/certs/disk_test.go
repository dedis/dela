package certs

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func TestDiskStore_Store(t *testing.T) {
	db, clean := makeDb(t)
	defer clean()

	cert0 := fake.MakeCertificate(t, 1, net.IPv4(127, 0, 0, 1))
	cert0.PrivateKey = nil
	cert12 := fake.MakeCertificate(t, 1, net.IPv4(127, 0, 0, 12))
	cert12.PrivateKey = nil

	store := NewDiskStore(db, fake.AddressFactory{})

	err := store.Store(fake.NewAddress(0), cert0)
	require.NoError(t, err)

	err = store.Store(fake.NewAddress(12), cert12)
	require.NoError(t, err)

	cert, err := store.InMemoryStore.Load(fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, cert0, cert)
	cert, err = store.InMemoryStore.Load(fake.NewAddress(12))
	require.NoError(t, err)
	require.Equal(t, cert12, cert)

	other := NewDiskStore(db, fake.AddressFactory{})

	cert, err = other.Load(fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, cert0, cert)
	cert, err = other.Load(fake.NewAddress(12))
	require.NoError(t, err)
	require.Equal(t, cert12, cert)
}

func TestDiskStore_BadKey_Store(t *testing.T) {
	store := &DiskStore{}

	err := store.Store(fake.NewBadAddress(), nil)
	require.EqualError(t, err, fake.Err("certificate key failed"))
}

func TestDiskStore_FailCreateBucket_Store(t *testing.T) {
	store := &DiskStore{
		db: fake.NewBadDB(),
	}

	err := store.Store(fake.NewAddress(0), fake.MakeCertificate(t, 0))
	require.EqualError(t, err, fake.Err("while updating db: while getting bucket"))
}

func TestDiskStore_FailWriteDB_Store(t *testing.T) {
	memdb := fake.NewInMemoryDB()
	memdb.SetBucket(certBucket, fake.NewBadWriteBucket())

	store := &DiskStore{
		bucket: certBucket,
		db:     memdb,
	}

	err := store.Store(fake.NewAddress(0), fake.MakeCertificate(t, 0))
	require.EqualError(t, err, fake.Err("while updating db: while writing"))
}

func TestDiskStore_Load(t *testing.T) {
	db, clean := makeDb(t)
	defer clean()

	cert := fake.MakeCertificate(t, 0)

	store := NewDiskStore(db, fake.AddressFactory{})

	err := store.Store(fake.NewAddress(0), cert)
	require.NoError(t, err)

	cert, err = store.Load(fake.NewAddress(0))
	require.NoError(t, err)
	require.NotNil(t, cert)

	cert, err = store.Load(fake.NewAddress(1))
	require.NoError(t, err)
	require.Nil(t, cert)

	store = NewDiskStore(fake.NewInMemoryDB(), fake.AddressFactory{})
	cert, err = store.Load(fake.NewAddress(0))
	require.NoError(t, err)
	require.Nil(t, cert)
}

func TestDiskStore_BadKey_Load(t *testing.T) {
	store := &DiskStore{
		InMemoryStore: NewInMemoryStore(),
	}

	_, err := store.Load(fake.NewBadAddress())
	require.EqualError(t, err, fake.Err("certificate key failed"))
}

func TestDiskStore_FailReadDB_Load(t *testing.T) {
	store := &DiskStore{
		InMemoryStore: NewInMemoryStore(),
		db:            fake.NewBadViewDB(),
	}

	_, err := store.Load(fake.NewAddress(0))
	require.EqualError(t, err, fake.Err("while reading db"))
}

func TestDiskStore_MalformedCert_Load(t *testing.T) {
	bucket := fake.NewBucket()
	key, err := fake.NewAddress(0).MarshalText()
	require.NoError(t, err)
	bucket.Set(key, []byte{1, 2, 3})

	memdb := fake.NewInMemoryDB()
	memdb.SetBucket(certBucket, bucket)

	store := &DiskStore{
		bucket:        certBucket,
		InMemoryStore: NewInMemoryStore(),
		db:            memdb,
	}

	_, err = store.Load(fake.NewAddress(0))
	require.Error(t, err)
	require.Contains(t, err.Error(), "certificate malformed: asn1: ")
}

func TestDiskStore_Delete(t *testing.T) {
	db, clean := makeDb(t)
	defer clean()

	cert := fake.MakeCertificate(t, 0)
	key := fake.NewAddress(0)

	store := NewDiskStore(db, fake.AddressFactory{})

	err := store.Delete(key)
	require.NoError(t, err)

	err = store.Store(key, cert)
	require.NoError(t, err)

	err = store.Delete(key)
	require.NoError(t, err)

	cert, err = store.Load(key)
	require.NoError(t, err)
	require.Nil(t, cert)

	store = NewDiskStore(db, fake.AddressFactory{})

	cert, err = store.Load(key)
	require.NoError(t, err)
	require.Nil(t, cert)
}

func TestDiskStore_BadKey_Delete(t *testing.T) {
	store := &DiskStore{
		InMemoryStore: NewInMemoryStore(),
	}

	err := store.Delete(fake.NewBadAddress())
	require.EqualError(t, err, fake.Err("certificate key failed"))
}

func TestDiskStore_FailWriteDB_Delete(t *testing.T) {
	memdb := fake.NewInMemoryDB()
	memdb.SetBucket(certBucket, fake.NewBadDeleteBucket())

	store := &DiskStore{
		InMemoryStore: NewInMemoryStore(),
		bucket:        certBucket,
		db:            memdb,
	}

	err := store.Delete(fake.NewAddress(0))
	require.EqualError(t, err, fake.Err("while updating db: while deleting"))
}

func TestDiskStore_Range(t *testing.T) {
	db, clean := makeDb(t)
	defer clean()

	store := NewDiskStore(db, fake.AddressFactory{})

	count := 0
	err := store.Range(func(addr mino.Address, cert *tls.Certificate) bool {
		count++
		return true
	})
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = store.Store(fake.NewAddress(0), fake.MakeCertificate(t, 0))
	require.NoError(t, err)
	err = store.Store(fake.NewAddress(1), fake.MakeCertificate(t, 0))
	require.NoError(t, err)
	err = store.Store(fake.NewAddress(2), fake.MakeCertificate(t, 0))
	require.NoError(t, err)

	err = store.Range(func(addr mino.Address, cert *tls.Certificate) bool {
		count++
		return true
	})
	require.NoError(t, err)
	require.Equal(t, 3, count)

	err = store.Range(func(addr mino.Address, cert *tls.Certificate) bool {
		count++
		return false
	})
	require.NoError(t, err)
	require.Equal(t, 4, count)
}

func TestDiskNode_MalformedCertificate_Range(t *testing.T) {
	bucket := fake.NewBucket()
	bucket.Set([]byte{}, []byte{1, 2, 3})

	memdb := fake.NewInMemoryDB()
	memdb.SetBucket(certBucket, bucket)

	store := &DiskStore{
		InMemoryStore: NewInMemoryStore(),
		bucket:        certBucket,
		db:            memdb,
	}

	err := store.Range(func(addr mino.Address, cert *tls.Certificate) bool { return true })
	require.Error(t, err)
	require.Contains(t, err.Error(), "while reading db: certificate malformed: asn1: ")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeDb(t *testing.T) (kv.DB, func()) {
	file, err := ioutil.TempFile(os.TempDir(), "minogrpc-certs")
	require.NoError(t, err)

	db, err := kv.New(file.Name())
	require.NoError(t, err)

	return db, func() { os.Remove(file.Name()) }
}
