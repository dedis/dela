package types

import (
	"bytes"
	"sync"

	"go.dedis.ch/dela/consensus"
	"golang.org/x/xerrors"
)

// Storage is the interface that defines the forward link storage.
type Storage interface {
	Len() uint64
	Store(ForwardLink) error
	StoreChain(consensus.Chain) error
	ReadChain(to Digest) (consensus.Chain, error)
}

type inMemoryStorage struct {
	sync.Mutex
	links []ForwardLink
}

func NewInMemoryStorage() Storage {
	return &inMemoryStorage{}
}

func (s *inMemoryStorage) Len() uint64 {
	return uint64(len(s.links))
}

// Store implements Storage. It appends the forward link to the storage if it
// matches the previous.
func (s *inMemoryStorage) Store(link ForwardLink) error {
	s.Lock()
	defer s.Unlock()

	if len(s.links) == 0 {
		s.links = append(s.links, link)
		return nil
	}

	last := s.links[len(s.links)-1]
	if !bytes.Equal(last.to, link.from) {
		return xerrors.Errorf("mismatch forward link '%x' != '%x'",
			last.to, link.from)
	}

	s.links = append(s.links, link)

	return nil
}

// StoreChain implements Storage. It updates the local chain if the given one is
// consistent.
func (s *inMemoryStorage) StoreChain(in consensus.Chain) error {
	chain, ok := in.(Chain)
	if !ok {
		return xerrors.Errorf("invalid chain type '%T'", in)
	}

	s.Lock()
	defer s.Unlock()

	for i, link := range chain.links {
		if i < len(s.links) {
			if !bytes.Equal(link.from, s.links[i].from) {
				return xerrors.Errorf("mismatch link %d: from '%#x' != '%#x'",
					i, link.from, s.links[i].from)
			}

			if !bytes.Equal(link.to, s.links[i].to) {
				return xerrors.Errorf("mismatch link %d: to '%#x' != '%#x'",
					i, link.to, s.links[i].to)
			}
		} else if len(s.links) > 0 {
			last := s.links[len(s.links)-1]
			if !bytes.Equal(last.to, link.from) {
				return xerrors.Errorf("couldn't append link: '%#x' != '%#x'",
					last.to, link.from)
			}

			s.links = append(s.links, link)
		} else {
			s.links = append(s.links, link)
		}
	}

	return nil
}

// ReadChain implements Storage. It returns the list of forward links to the id.
// It returns an error if the target is never reached. If the id is nil, it will
// return the whole chain.
func (s *inMemoryStorage) ReadChain(id Digest) (consensus.Chain, error) {
	s.Lock()
	defer s.Unlock()

	links := make([]ForwardLink, 0)
	for _, link := range s.links {
		links = append(links, link)
		if bytes.Equal(link.to, id) {
			return Chain{links: links}, nil
		}
	}

	if id != nil {
		return nil, xerrors.Errorf("id '%x' not found", id)
	}

	return Chain{links: links}, nil
}
