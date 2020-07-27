package blockstore

import (
	"bytes"

	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"golang.org/x/xerrors"
)

type InMemory struct {
	blocks []types.BlockLink
}

func NewInMemory() *InMemory {
	return &InMemory{
		blocks: make([]types.BlockLink, 0),
	}
}

func (s *InMemory) Len() int {
	return len(s.blocks)
}

func (s *InMemory) Store(link types.BlockLink) error {
	if len(s.blocks) > 0 {
		latest := s.blocks[len(s.blocks)-1]

		if !bytes.Equal(latest.GetTo().GetHash(), link.GetFrom()) {
			return xerrors.Errorf("mismatch link")
		}
	}

	s.blocks = append(s.blocks, link)

	return nil
}

func (s *InMemory) Get(id []byte) (types.BlockLink, error) {
	for _, link := range s.blocks {
		if bytes.Equal(link.GetTo().GetHash(), id) {
			return link, nil
		}
	}

	return types.BlockLink{}, xerrors.New(ErrBlockUnknown)
}

func (s *InMemory) Last() (types.BlockLink, error) {
	if len(s.blocks) == 0 {
		return types.BlockLink{}, xerrors.New(ErrBlockUnknown)
	}

	return s.blocks[len(s.blocks)-1], nil
}
