// This file contains an implementation of a certificate storage on the disk
// which enables persistence.
//
// Documentation Last Review: 07.10.2020
//

package certs

import (
	"errors"

	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

var certBucket = []byte("certificates")

var errInterrupt = errors.New("interrupted")

// DiskStore is a persistent implementation of a certificate storage. It uses
// internally an in-memory store to cache the certificates.
//
// - implements certs.Storage
type DiskStore struct {
	*InMemoryStore

	db      kv.DB
	bucket  []byte
	addrFac mino.AddressFactory
}

// NewDiskStore returns a new empty disk store. If certificates are stored in
// the database, they will be loaded on demand.
func NewDiskStore(db kv.DB, fac mino.AddressFactory) *DiskStore {
	return &DiskStore{
		InMemoryStore: NewInMemoryStore(),
		db:            db,
		bucket:        certBucket,
		addrFac:       fac,
	}
}

// Store implements certs.Storage. It stores the certificate in the disk and in
// the cache.
func (s *DiskStore) Store(addr mino.Address, chain CertChain) error {
	key, err := addr.MarshalText()
	if err != nil {
		return xerrors.Errorf("certificate key failed: %v", err)
	}

	// Save the certificate in the disk so that it can later be retrieved.
	err = s.db.Update(func(tx kv.WritableTx) error {
		bucket, err := tx.GetBucketOrCreate(s.bucket)
		if err != nil {
			return xerrors.Errorf("while getting bucket: %v", err)
		}

		err = bucket.Set(key, chain)
		if err != nil {
			return xerrors.Errorf("while writing: %v", err)
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("while updating db: %v", err)
	}

	s.InMemoryStore.Store(addr, chain)

	return nil
}

// Load implements certs.Storage. It first tries to read the certificate from
// the cache, then from the disk. It returns nil if not found in both.
func (s *DiskStore) Load(addr mino.Address) (CertChain, error) {
	cached, _ := s.InMemoryStore.Load(addr)
	if cached != nil {
		return cached, nil
	}

	key, err := addr.MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("certificate key failed: %v", err)
	}

	var chain CertChain

	err = s.db.View(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(s.bucket)
		if bucket == nil {
			return nil
		}

		value := bucket.Get(key)

		if len(value) == 0 {
			return nil
		}

		data := make([]byte, len(value))
		copy(data, value)

		chain = data

		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("while reading db: %v", err)
	}

	// Keep the certificate in cache for faster access.
	s.InMemoryStore.Store(addr, chain)

	return chain, nil
}

// Delete implements certs.Storage. It deletes the certificate from the disk and
// the cache.
func (s *DiskStore) Delete(addr mino.Address) error {
	s.InMemoryStore.Delete(addr)

	key, err := addr.MarshalText()
	if err != nil {
		return xerrors.Errorf("certificate key failed: %v", err)
	}

	err = s.db.Update(func(tx kv.WritableTx) error {
		bucket := tx.GetBucket(s.bucket)
		if bucket == nil {
			return nil
		}

		err := bucket.Delete(key)
		if err != nil {
			return xerrors.Errorf("while deleting: %v", err)
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("while updating db: %v", err)
	}

	return nil
}

// Range implements certs.Storage. It iterates over each certificate present in
// the disk.
func (s *DiskStore) Range(fn func(mino.Address, CertChain) bool) error {
	err := s.db.View(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(s.bucket)
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(key, value []byte) error {
			// The raw certificate is retained in the x509 leaf, therefore it
			// needs an independent array. The database could reuse the one
			// provided.
			data := make([]byte, len(value))
			copy(data, value)

			addr := s.addrFac.FromText(key)

			next := fn(addr, data)
			if !next {
				return errInterrupt
			}

			return nil
		})
	})
	if errors.Is(err, errInterrupt) {
		// The iteration is interrupted by the caller, so that is not a real
		// error.
		return nil
	}
	if err != nil {
		return xerrors.Errorf("while reading db: %v", err)
	}

	return nil
}
