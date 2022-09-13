//
// Documentation Last Review: 06.10.2020
//

package tree

import (
	"math"
	"sync"

	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

const minNumChildren = float64(5)

// Tree is the interface used by the router to determine the routes.
type Tree interface {
	// GetMaxHeight returns the maximum height for this tree.
	GetMaxHeight() int

	// GetRoute returns the address to route the provided target. It will return
	// a nil value if no route is found in this tree.
	GetRoute(to mino.Address) (mino.Address, error)

	// GetChildren returns the children of a direct branch of this tree. It
	// represents the list of routable addresses for a given branch.
	GetChildren(to mino.Address) []mino.Address

	// Remove marks the address as unreachable and fixes the tree if
	// appropriate.
	Remove(addr mino.Address)
}

// AddrSet is a set of unique addresses.
type AddrSet map[mino.Address]struct{}

// Search returns true if the provided address is in the set.
func (set AddrSet) Search(to mino.Address) bool {
	_, found := set[to]
	return found
}

// GetRandom returns a member of the set randomly.
func (set AddrSet) GetRandom() mino.Address {
	for addr := range set {
		return addr
	}

	return nil
}

// Branches is a partial representation of a tree which shows only the direct
// branches of the node and children's branch, but unstructured.
type Branches map[mino.Address]AddrSet

// Search returns the address of the direct branch that leads to the provided
// address if any, otherwise it will return nil.
func (c Branches) Search(to mino.Address) mino.Address {
	for gateway, set := range c {
		if gateway.Equal(to) || set.Search(to) {
			return gateway
		}
	}

	return nil
}

// DynamicTree is a tree that will optimistically build the tree according to
// route requests. It will adjust the branches and their children when
// necessary.
//
// The tree is built according to the theory of m-ary trees to find the minimum
// value of m in order to have a fixed maximum depth. The structure itself is
// not the complete tree but only the first level of branches with their
// unstructured children. Each router is responsible to build its own level.
//
// - implements tree.Tree
type dynTree struct {
	sync.Mutex
	height   int
	m        int
	branches Branches
	expected AddrSet
	offline  AddrSet
}

// NewTree creates a new empty tree that will spawn to a maximum depth and route
// only the given addresses.
func NewTree(height int, addrs []mino.Address) Tree {
	N := float64(len(addrs))
	// m finds the minimum number of branches needed to not go deeper than the
	// given height.
	m := math.Exp(math.Log(N) / float64(height))

	// ... but we use a minimal value to avoid unnecessary deep trees.
	m = math.Max(m, minNumChildren)

	expected := make(AddrSet)
	for _, addr := range addrs {
		expected[addr] = struct{}{}
	}

	return &dynTree{
		height:   height,
		m:        int(m),
		branches: make(Branches),
		expected: expected,
		offline:  make(AddrSet),
	}
}

// GetMaxHeight implements tree.Tree. It returns the maximum depth for this
// tree.
func (t *dynTree) GetMaxHeight() int {
	return t.height
}

// GetRoute implements tree.Tree. It returns the address to route the target, or
// nil if no route is found.
func (t *dynTree) GetRoute(to mino.Address) (mino.Address, error) {
	t.Lock()
	defer t.Unlock()

	if t.offline.Search(to) {
		return nil, xerrors.Errorf("address is unreachable")
	}

	gateway := t.branches.Search(to)
	if gateway != nil {
		return gateway, nil
	}

	if t.expected.Search(to) {
		// Add the address as a branch of the tree and optimistically attribute
		// it some children.
		t.updateTree(to)

		return to, nil
	}

	return nil, nil
}

// GetChildren implements tree.Tree. It returns the children of a branch.
func (t *dynTree) GetChildren(to mino.Address) []mino.Address {
	t.Lock()
	defer t.Unlock()

	set := t.branches[to]
	addrs := make([]mino.Address, 0, len(set))

	for addr := range set {
		addrs = append(addrs, addr)
	}

	return addrs
}

// Remove implements tree.Tree. It marks the address as unreachable if it is an
// known one. It also makes sure the address is not a branch, otherwise it uses
// one of the child to route the packets instead.
func (t *dynTree) Remove(addr mino.Address) {
	t.Lock()
	defer t.Unlock()

	if t.expected.Search(addr) || t.branches.Search(addr) != nil {
		// If the address is supposed to be routed by the tree, it ends up
		// in the list of unreachable addresses.
		t.offline[addr] = struct{}{}
	}

	// It is also necessary to make sure the address is not a branch, otherwise
	// it needs to be replaced.
	branch, found := t.branches[addr]
	if !found {
		return
	}

	delete(t.branches, addr)

	// Pick a random child and grant it the parent role.
	newParent := branch.GetRandom()
	if newParent == nil {
		return
	}

	delete(branch, newParent)
	t.branches[newParent] = branch
}

func (t *dynTree) updateTree(to mino.Address) {
	// Remove the direct child from the list of waiting peers.
	delete(t.expected, to)

	// Find how many children this direct child should have.
	remain := t.m - len(t.branches)
	num := math.Ceil(float64(len(t.expected)-remain+1) / float64(remain))

	set := make(AddrSet)
	for addr := range t.expected {
		if len(set) >= int(num) {
			break
		}

		set[addr] = struct{}{}
		delete(t.expected, addr)
	}

	// Optimistic creation of a branch for this node. It assumes that none of
	// the thoses addresses will come before the branches are created but this
	// is not true. The tree will correct itself if that happens.
	// TODO: auto-update by updates in types.Packet
	t.branches[to] = set
}
