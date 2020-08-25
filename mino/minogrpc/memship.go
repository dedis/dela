package minogrpc

import (
	"go.dedis.ch/dela/mino"
)

// Memship is a dummy membership service
//
// - implements tree.MembershipService
type Memship struct {
	addrs []mino.Address
}

// NewMemship creates a new memship
func NewMemship(addrs []mino.Address) *Memship {
	return &Memship{
		addrs: addrs,
	}
}

// Get implements tree.MembershipService
func (m Memship) Get(id []byte) []mino.Address {
	return m.addrs
}

// Add adds a new address to the memship service
func (m *Memship) Add(addr mino.Address) {
	m.addrs = append(m.addrs, addr)
}
