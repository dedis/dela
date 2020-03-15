package qsc

import (
	fmt "fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestTLCR_Basic(t *testing.T) {
	n := 5
	k := 5
	bcs := makeTLCR(t, n)

	wg := sync.WaitGroup{}
	wg.Add(n)
	for _, bc := range bcs {
		go func(bc *bTLCR) {
			for i := 0; i < k; i++ {
				bc.execute(&Message{})
			}
			wg.Done()
		}(bc)
	}

	wg.Wait()
	println("Done.. !")

	require.Equal(t, uint64(k), bcs[0].timeStep)
}

func makeTLCR(t *testing.T, n int) []*bTLCR {
	manager := minoch.NewManager()
	players := &fakePlayers{}
	bcs := make([]*bTLCR, n)
	for i := range bcs {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		bc, err := newTLCR(m, players)
		require.NoError(t, err)
		bc.node = int64(i)

		bcs[i] = bc
	}

	return bcs
}

type fakeIterator struct {
	index int
	addrs []mino.Address
}

func (i *fakeIterator) HasNext() bool {
	return i.index+1 < len(i.addrs)
}

func (i *fakeIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.addrs[i.index]
	}
	return nil
}

type fakePlayers struct {
	addrs []mino.Address
}

func (p *fakePlayers) AddressIterator() mino.AddressIterator {
	return &fakeIterator{addrs: p.addrs, index: -1}
}

func (p *fakePlayers) Len() int {
	return len(p.addrs)
}
