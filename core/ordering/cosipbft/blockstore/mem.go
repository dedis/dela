package blockstore

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"golang.org/x/xerrors"
)

// InMemory is a block store that only stores the block in-memory which means
// they won't persist.
//
// - implements blockstore.BlockStore
type InMemory struct {
	blocks []types.BlockLink
}

// NewInMemory returns a new empty in-memory block store.
func NewInMemory() *InMemory {
	return &InMemory{
		blocks: make([]types.BlockLink, 0),
	}
}

// Len implements blockstore.BlockStore. It returns the length of the store.
func (s *InMemory) Len() int {
	return len(s.blocks)
}

// Store implements blockstore.BlockStore. It stores the block only if the link
// matches the latest block.
func (s *InMemory) Store(link types.BlockLink) error {
	if len(s.blocks) > 0 {
		latest := s.blocks[len(s.blocks)-1]

		if latest.GetTo().GetHash() != link.GetFrom() {
			return xerrors.Errorf("mismatch link '%v' != '%v'",
				link.GetFrom(), latest.GetTo().GetHash())
		}
	}

	s.blocks = append(s.blocks, link)

	return nil
}

// Get implements blockstore.BlockStore. It returns the block link associated to
// the digest if it exists, otherwise it returns an error.
func (s *InMemory) Get(id types.Digest) (types.BlockLink, error) {
	for _, link := range s.blocks {
		if link.GetTo().GetHash() == id {
			return link, nil
		}
	}

	return types.BlockLink{}, xerrors.Errorf("block not found: %w", ErrNoBlock)
}

// Last implements blockstore.BlockStore. It returns the latest block of the
// store.
func (s *InMemory) Last() (types.BlockLink, error) {
	if len(s.blocks) == 0 {
		return types.BlockLink{}, xerrors.Errorf("store empty: %w", ErrNoBlock)
	}

	return s.blocks[len(s.blocks)-1], nil
}
