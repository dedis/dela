package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

func ExampleLoader_LoadOrCreate() {
	dir, err := os.MkdirTemp(os.TempDir(), "example")
	if err != nil {
		panic("no folder: " + err.Error())
	}

	defer os.RemoveAll(dir)

	loader := NewFileLoader(filepath.Join(dir, "private.key"))

	data, err := loader.LoadOrCreate(exampleGenerator{})
	if err != nil {
		panic("loading key failed: " + err.Error())
	}

	fmt.Println(string(data))

	// Output: a marshaled private key
}

// Example of dummy generator which generates a key and returns its
// serialization.
//
// - implements loader.Generator
type exampleGenerator struct{}

// Generate implements loader.Generator. It returns a dummy byte slice for the
// example.
func (exampleGenerator) Generate() ([]byte, error) {
	return []byte("a marshaled private key"), nil
}
