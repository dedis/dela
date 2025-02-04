package key

import (
	"crypto/rand"

	"github.com/libp2p/go-libp2p/core/crypto"
	"go.dedis.ch/dela/core/store/kv"
	"golang.org/x/xerrors"
)

// Storage provides persistent Storage for private keys.
type Storage struct {
	bucket []byte
	db     kv.DB
}

// NewStorage creates a new Storage for private keys.
func NewStorage(db kv.DB) *Storage {
	return &Storage{
		bucket: []byte("minows_keys"),
		db:     db,
	}
}

// LoadOrCreate loads the private key from Storage or
// creates a new one if none exists.
func (s *Storage) LoadOrCreate() (crypto.PrivKey, error) {
	key := []byte("private_key")
	var buffer []byte
	err := s.db.Update(func(tx kv.WritableTx) error {
		bucket, err := tx.GetBucketOrCreate(s.bucket)
		if err != nil {
			return xerrors.Errorf("could not get bucket: %v", err)
		}

		bytes := bucket.Get(key)
		if bytes == nil {
			private, _, err := crypto.GenerateEd25519Key(rand.Reader)
			if err != nil {
				return xerrors.Errorf("could not generate key: %v", err)
			}
			bytes, err = crypto.MarshalPrivateKey(private)
			if err != nil {
				return xerrors.Errorf("could not marshal key: %v", err)
			}
			err = bucket.Set(key, bytes)
			if err != nil {
				return xerrors.Errorf("could not store key: %v", err)
			}
		}
		buffer = make([]byte, len(bytes))
		copy(buffer, bytes)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("could not update db: %v", err)
	}

	private, err := crypto.UnmarshalPrivateKey(buffer)
	if err != nil {
		return nil, xerrors.Errorf("could not unmarshal key: %v", err)
	}
	return private, nil
}
