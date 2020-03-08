package cosipbft

import (
	"bytes"
	"sync"

	"golang.org/x/xerrors"
)

// Storage is the interface that defines the forward link storage.
type Storage interface {
	Store(*ForwardLinkProto) error
	ReadLast() (*ForwardLinkProto, error)
	ReadChain(id Digest) ([]*ForwardLinkProto, error)
}

type inMemoryStorage struct {
	sync.Mutex
	links []*ForwardLinkProto
}

func newInMemoryStorage() *inMemoryStorage {
	return &inMemoryStorage{}
}

// Store appends the forward link to the storage if it matches the previous.
func (s *inMemoryStorage) Store(link *ForwardLinkProto) error {
	s.Lock()
	defer s.Unlock()

	if len(s.links) == 0 {
		s.links = append(s.links, link)
		return nil
	}

	last := s.links[len(s.links)-1]
	if !bytes.Equal(last.GetTo(), link.GetFrom()) {
		return xerrors.Errorf("mismatch forward link '%x' != '%x'",
			last.GetTo(), link.GetFrom())
	}

	s.links = append(s.links, link)

	return nil
}

// ReadLast returns the last forward link stored or nil if it doesn't exist.
func (s *inMemoryStorage) ReadLast() (*ForwardLinkProto, error) {
	s.Lock()
	defer s.Unlock()

	if len(s.links) == 0 {
		return nil, nil
	}

	return s.links[len(s.links)-1], nil
}

// ReadChain returns the list of forward links to the id. It returns an error
// if the target is never reached.
func (s *inMemoryStorage) ReadChain(id Digest) ([]*ForwardLinkProto, error) {
	s.Lock()
	defer s.Unlock()

	links := make([]*ForwardLinkProto, 0)
	for _, link := range s.links {
		links = append(links, link)
		if bytes.Equal(link.GetTo(), id) {
			return links, nil
		}
	}

	return nil, xerrors.Errorf("id '%x' not found", id)
}
