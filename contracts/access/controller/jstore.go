package controller

import (
	"io/ioutil"
	"os"
	"sync"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

// accessStore defines a simple read/write interface to store the access
type accessStore interface {
	store.Writable
	store.Readable
}

func newJstore(path string) (accessStore, error) {
	data := map[string][]byte{}

	ctx := json.NewContext()

	if fileExist(path) {
		buf, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, xerrors.Errorf("failed to read file '%s': %v", path, err)
		}

		err = ctx.Unmarshal(buf, &data)
		if err != nil {
			return nil, xerrors.Errorf("failed to read json: %v", err)
		}
	}

	return &jstore{
		ctx:  ctx,
		path: path,
		data: data,
	}, nil
}

// jstore implements a simple store to store accesses on the access contract. It
// keeps the data in memory AND in a json file.
//
// - implements accessStore
type jstore struct {
	sync.Mutex

	ctx serde.Context

	path string
	data map[string][]byte
}

func (s *jstore) Set(key []byte, value []byte) error {
	s.Lock()
	defer s.Unlock()

	s.data[string(key)] = value
	s.saveFile()

	return nil
}

func (s *jstore) Delete(key []byte) error {
	s.Lock()
	defer s.Unlock()

	delete(s.data, string(key))
	s.saveFile()

	return nil
}

// return a nil value if not found
func (s *jstore) Get(key []byte) ([]byte, error) {
	s.Lock()
	defer s.Unlock()

	return s.data[string(key)], nil
}

func (s *jstore) saveFile() error {
	buf, err := s.ctx.Marshal(s.data)
	if err != nil {
		return xerrors.Errorf("failed to marshal data: %v", err)
	}

	err = ioutil.WriteFile(s.path, buf, os.ModePerm)
	if err != nil {
		return xerrors.Errorf("failed to save file '%s': %v", s.path, err)
	}

	return nil
}

func fileExist(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
