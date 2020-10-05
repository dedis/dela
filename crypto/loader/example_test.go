package loader

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func ExampleLoader_LoadOrCreate() {
	dir, err := ioutil.TempDir(os.TempDir(), "example")
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

type exampleGenerator struct{}

func (exampleGenerator) Generate() ([]byte, error) {
	return []byte("a marshaled private key"), nil
}
