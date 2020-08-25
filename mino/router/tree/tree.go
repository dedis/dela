package tree

import (
	"bytes"
	"fmt"
	"io"
	"math"

	"go.dedis.ch/dela/mino"
)

// buildTree builds the newtwork tree based on the list of addresses. The first
// call should have an index of 0. The height is the maximum number of edges in
// the longest path from the root to a leaf.
func buildTree(addr mino.Address, addrs []mino.Address, h, index int,
	parent *treeNode) *treeNode {
	// the height can not be higher than the total number of nodes, ie. if there
	// are 10 nodes, then the maximum height we can have is 9. Here len(addrs)
	// represents the number of nodes-1 because the root addr is not included.
	if h > len(addrs) {
		h = len(addrs)
	}

	node := &treeNode{
		Index:     index,
		Addr:      addr,
		LastIndex: index + len(addrs),
		Parent:    parent,
	}

	children := make([]*treeNode, 0)

	// N: total number of nodes
	// D: number of direct connections wanted for each node
	// H: height of the network tree
	//
	// N = D^(H+1) - 1
	// D = sqrt[H+1](N+1)
	// H = log_D(N+1) - 1

	N := float64(len(addrs) + 1)
	d := int(math.Round(math.Pow(N+1.0, 1.0/float64(h+1))))

	// This is a check that with the computed number of neighbors there will be
	// enough nodes to reach the given height. For example, if there are 6
	// addresses in the list, H = 4, then D = 1.51, which will be rounded to 2.
	// However, we can see that if we split the list in two, there will be 3
	// addresses in each sub-list that will have to reach a height of H-1 = 3,
	// which is impossible with 3 addresses. So this is why we decrement D.
	if len(addrs)/d < h {
		d = d - 1
	}

	// If we must build a height of 1 there is no other solutions than having
	// all the addresses as children.
	if h == 1 {
		d = len(addrs)
	}

	// k is the total number of elements in a sub tree
	//
	// In the case we want 2 direct connection per node and we have 7 addresses,
	// we then split the list into two parts, and there will be 3.5 addresses in
	// each part, re-arranged into 3 || 4:
	// a1 a2 a3 | a4 a5 a6 a7
	// "a1" will be the root of the first part
	// and "a4" the second one. We use k to delimit each part with k*i.
	k := float64(float64(len(addrs))) / float64(d)

	if k == 0 {
		children = []*treeNode{}
	} else if k < 1 {
		// This is the last level
		for i := 0; i < len(addrs); i++ {
			child := buildTree(addrs[i], []mino.Address{}, h-1, index+i+1, node)
			children = append(children, child)
		}
	} else {
		for i := 0; i < d; i++ {
			firstI := int(k * float64(i))
			lastI := int(k*float64(i) + k)
			child := buildTree(addrs[firstI], addrs[firstI+1:lastI], h-1,
				1+index+firstI, node)
			children = append(children, child)
		}
	}

	node.Children = children

	return node
}

// treeNode represents the address of a network node and the direct connections
// this network node has, represented by its children. The Index and LastIndex
// denotes the range of addresses the node has in its sub-tree.
type treeNode struct {
	Index     int
	LastIndex int
	Addr      mino.Address
	Children  []*treeNode
	Parent    *treeNode
}

func (n treeNode) Display(out io.Writer) {
	fmt.Fprintf(out, "Node[%s-index[%d]-lastIndex[%d]](",
		n.Addr.String(), n.Index, n.LastIndex)

	if len(n.Children) > 0 {
		fmt.Fprintf(out, "\n")
	}

	for _, c := range n.Children {
		var buf bytes.Buffer
		c.Display(&buf)
		fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "\t$1"))
	}

	fmt.Fprint(out, ")\n")
}

// ForEach calls f on each node in a depth-first pre-order manner
func (n *treeNode) ForEach(f func(n *treeNode)) {
	f(n)
	for _, c := range n.Children {
		c.ForEach(f)
	}
}
