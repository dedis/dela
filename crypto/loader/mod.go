// Package loader defines an abstraction to store a private, or a public, key in
// a storage.
//
// When the key does not exist, it will generate a new one using a generator
// implemented by the caller and stores it for the next time.
//
// Documentation Last Review: 06.10.2020
//
package loader

// Generator is the interface to implement to generate a key.
type Generator interface {
	Generate() ([]byte, error)
}

// Loader is an abstraction to load a key from a storage. It allows for instance
// to load a private key from the disk, or generate it if it doesn't exist.
type Loader interface {
	// LoadOrCreate tries to load the key and returns it if found, otherwise it
	// generates a new one using the generator and stores it.
	LoadOrCreate(Generator) ([]byte, error)

	// Load loads the key and returns an error if it doesn't find it.
	Load() ([]byte, error)
}
