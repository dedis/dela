package cosipbft

import (
	"bytes"
	"sync"

	"go.dedis.ch/dela/consensus"
	"golang.org/x/xerrors"
)

// Storage is the interface that defines the forward link storage.
type Storage interface {
	Store(forwardLink) error
	StoreChain(consensus.Chain) error
	ReadChain(to Digest) (consensus.Chain, error)
}

type inMemoryStorage struct {
	sync.Mutex
	links []forwardLink
}

func newInMemoryStorage() *inMemoryStorage {
	return &inMemoryStorage{}
}

// Store appends the forward link to the storage if it matches the previous.
func (s *inMemoryStorage) Store(link forwardLink) error {
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

func (s *inMemoryStorage) StoreChain(in consensus.Chain) error {
	chain, ok := in.(forwardLinkChain)
	if !ok {
		return xerrors.New("invalid chain type")
	}

	s.Lock()
	defer s.Unlock()

	for i, link := range chain.links {
		if i < len(s.links) {
			if !bytes.Equal(link.from, s.links[i].from) || !bytes.Equal(link.to, s.links[i].to) {
				return xerrors.New("chain is inconsistent")
			}
		} else if len(s.links) > 0 {
			last := s.links[len(s.links)-1]
			if !bytes.Equal(last.to, link.from) {
				return xerrors.New("chain is inconsistent")
			}

			s.links = append(s.links, link)
		} else {
			s.links = append(s.links, link)
		}
	}

	return nil
}

// ReadChain returns the list of forward links to the id. It returns an error if
// the target is never reached.
func (s *inMemoryStorage) ReadChain(id Digest) (consensus.Chain, error) {
	s.Lock()
	defer s.Unlock()

	links := make([]forwardLink, 0)
	for _, link := range s.links {
		links = append(links, link)
		if bytes.Equal(link.to, id) {
			return forwardLinkChain{links: links}, nil
		}
	}

	if id != nil {
		return nil, xerrors.Errorf("id '%x' not found", id)
	}

	return forwardLinkChain{links: links}, nil
}
