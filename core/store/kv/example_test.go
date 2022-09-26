package kv

import (
	"fmt"
	"os"
	"path/filepath"
)

func ExampleBucket_Scan() {
	dir, err := os.MkdirTemp(os.TempDir(), "example")
	if err != nil {
		panic("failed to create folder: " + err.Error())
	}

	defer os.RemoveAll(dir)

	db, err := New(filepath.Join(dir, "example.db"))
	if err != nil {
		panic("failed to open db: " + err.Error())
	}

	pairs := [][]byte{
		{0b0101},
		{0b1111},
		{0b0000},
		{0b1110},
	}

	err = db.Update(func(tx WritableTx) error {
		bucket, err := tx.GetBucketOrCreate([]byte("example_bucket"))
		if err != nil {
			return err
		}

		for _, pair := range pairs {
			err = bucket.Set(pair, pair)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		panic("database write failed: " + err.Error())
	}

	err = db.View(func(tx ReadableTx) error {
		bucket := tx.GetBucket([]byte("example_bucket"))
		if bucket == nil {
			return nil
		}

		return bucket.Scan(nil, func(key, value []byte) error {
			fmt.Printf("%04b", key)
			fmt.Println("")
			return nil
		})
	})
	if err != nil {
		panic("database read failed: " + err.Error())
	}

	// Output: [0000]
	// [0101]
	// [1110]
	// [1111]
}
