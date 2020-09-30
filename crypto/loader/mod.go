package loader

// Generator is the interface to implement to generate a key if it does not
// exist.
type Generator interface {
	Generate() ([]byte, error)
}

// Loader is an abstraction to load a key from a storage. It allows for instance
// to load a private key from the disk.
type Loader interface {
	// LoadOrCreate tries to load the key and returns it if found, otherwise it
	// generates a new one using the generator and stores it.
	LoadOrCreate(Generator) ([]byte, error)
}
