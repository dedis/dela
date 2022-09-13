//
// Documentation Last Review: 06.10.2020
//

package loader

import (
	"io"
	"os"

	"golang.org/x/xerrors"
)

// FileLoader is loader that is storing the new keys to a file.
//
// - implements loader.Loader
type fileLoader struct {
	path string

	openFn     func(path string) (*os.File, error)
	openFileFn func(path string, flags int, perms os.FileMode) (*os.File, error)
	statFn     func(path string) (os.FileInfo, error)
}

// NewFileLoader creates a new loader that is using the file given in parameter.
func NewFileLoader(path string) Loader {
	return fileLoader{
		path:       path,
		openFn:     os.Open,
		openFileFn: os.OpenFile,
		statFn:     os.Stat,
	}
}

// LoadOrCreate implements loader.Loader. It either loads the key from the file
// if it exists, or it generates a new one and stores it in the file. The file
// created has minimal read permission for the current user (0400).
func (l fileLoader) LoadOrCreate(g Generator) ([]byte, error) {
	_, err := l.statFn(l.path)
	if os.IsNotExist(err) {
		data, err := g.Generate()
		if err != nil {
			return nil, xerrors.Errorf("generator failed: %v", err)
		}

		file, err := l.openFileFn(l.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0400)
		if err != nil {
			return nil, xerrors.Errorf("while creating file: %v", err)
		}

		defer file.Close()

		_, err = file.Write(data)
		if err != nil {
			return nil, xerrors.Errorf("while writing: %v", err)
		}

		return data, nil
	}

	data, err := l.Load()
	if err != nil {
		return nil, xerrors.Errorf("failed to load file: %v", err)
	}

	return data, nil
}

// Load implements loader.Loader. It loads the key from the file if it exists,
// otherwise it returns an error.
func (l fileLoader) Load() ([]byte, error) {
	file, err := l.openFn(l.path)
	if err != nil {
		return nil, xerrors.Errorf("while opening file: %v", err)
	}

	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, xerrors.Errorf("while reading file: %v", err)
	}

	return data, nil
}
