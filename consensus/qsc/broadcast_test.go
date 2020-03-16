package qsc

import (
	fmt "fmt"
	"sync"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
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
				bc.execute(&Message{Node: bc.node})
			}
			wg.Done()
		}(bc)
	}

	wg.Wait()

	require.Equal(t, uint64(k), bcs[0].timeStep)
}

func TestTLCB_Basic(t *testing.T) {
	n := 3
	k := 5

	bcs := makeTLCB(t, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for _, bc := range bcs {
		go func(bc *bTLCB) {
			defer wg.Done()
			var view *View
			var err error
			for i := 0; i < k; i++ {
				view, err = bc.execute(&empty.Empty{})
				require.NoError(t, err)
				require.Len(t, view.GetBroadcasted(), n)
				require.Len(t, view.GetReceived(), n)
			}
		}(bc)
	}

	wg.Wait()
}

func makeTLCR(t *testing.T, n int) []*bTLCR {
	manager := minoch.NewManager()
	players := &fakePlayers{}
	bcs := make([]*bTLCR, n)
	for i := range bcs {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		bc, err := newTLCR("tlcr", int64(i), m, players)
		require.NoError(t, err)

		bcs[i] = bc
	}

	return bcs
}

func makeTLCB(t *testing.T, n int) []*bTLCB {
	manager := minoch.NewManager()
	players := &fakePlayers{}
	bcs := make([]*bTLCB, n)
	for i := range bcs {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		bc, err := newTLCB(int64(i), m, players)
		require.NoError(t, err)

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

func (p *fakePlayers) SubSet(from, to int) mino.Players {
	return &fakePlayers{addrs: p.addrs[from:to]}
}

func (p *fakePlayers) AddressIterator() mino.AddressIterator {
	return &fakeIterator{addrs: p.addrs, index: -1}
}

func (p *fakePlayers) Len() int {
	return len(p.addrs)
}
