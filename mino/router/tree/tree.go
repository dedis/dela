package tree

import (
	"fmt"
	"io"
	"math"

	"go.dedis.ch/dela/mino"
)

const minNumChildren = float64(5)

type AddrSet map[mino.Address]struct{}

type Children map[mino.Address]AddrSet

func (c Children) Search(to mino.Address) mino.Address {
	for gateway, set := range c {
		if gateway.Equal(to) {
			return gateway
		}

		_, found := set[to]
		if found {
			return gateway
		}
	}

	return nil
}

type DynamicTree struct {
	height   int
	children Children
}

func NewTree(height int, expected []mino.Address) DynamicTree {
	N := float64(len(expected))
	// m finds the minimum number of children needed to not go deeper than the
	// given height.
	m := math.Exp(math.Log(N) / float64(height))

	// ... but we use a minimal value to avoid unnecessary deep trees.
	m = math.Max(m, minNumChildren)

	// It makes sure that we don't a value bigger than the length of the
	// addresses.
	num := int(math.Min(N, m))

	children := make(Children)
	for _, child := range expected[:num] {
		children[child] = make(AddrSet)
	}

	cursor := 0
	for _, addr := range expected[num:] {
		children[expected[cursor]][addr] = struct{}{}

		cursor = (cursor + 1) % num
	}

	return DynamicTree{
		height:   height,
		children: children,
	}
}

func (t DynamicTree) GetHeight() int {
	return t.height
}

func (t DynamicTree) GetRoute(to mino.Address) mino.Address {
	return t.children.Search(to)
}

func (t DynamicTree) GetAddressSet(to mino.Address) []mino.Address {
	set := t.children[to]
	addrs := make([]mino.Address, 0, len(set))

	for addr := range set {
		addrs = append(addrs, addr)
	}

	return addrs
}

func (t DynamicTree) Display(out io.Writer) {
	fmt.Fprintf(out, "Node\n")

	for gateway, children := range t.children {
		fmt.Fprintf(out, "-%v\n", gateway)

		for child := range children {
			fmt.Fprintf(out, "---%v\n", child)
		}
	}
}
