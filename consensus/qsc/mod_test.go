package qsc

import (
	fmt "fmt"
	"sync"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestQSC_Basic(t *testing.T) {
	n := 5
	k := 10

	val := &fakeValidator{}
	val.wg.Add(n * k)

	cons := makeQSC(t, n)
	actors := make([]consensus.Actor, n)
	for i, c := range cons {
		actor, err := c.Listen(val)
		require.NoError(t, err)

		actors[i] = actor
	}

	wg := sync.WaitGroup{}
	wg.Add(n)
	for j := 0; j < n; j++ {
		c := cons[j]
		actor := actors[j]
		go func() {
			defer wg.Done()
			for i := 0; i < k; i++ {
				err := actor.Propose(nil, nil)
				require.NoError(t, err)
			}
			close(c.ch)
		}()
	}

	wg.Wait()

	require.Equal(t, cons[0].history, cons[1].history)
	require.Len(t, cons[0].history, k)
}

func makeQSC(t *testing.T, n int) []*Consensus {
	manager := minoch.NewManager()
	cons := make([]*Consensus, n)
	players := &fakePlayers{}
	for i := range cons {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		qsc, err := NewQSC(int64(i), m, players)
		require.NoError(t, err)

		cons[i] = qsc
	}

	return cons
}

type fakeValidator struct {
	consensus.Validator
	wg sync.WaitGroup
}

func (v *fakeValidator) Validate(pb proto.Message) (consensus.Proposal, error) {
	return nil, nil
}

func (v *fakeValidator) Commit(id []byte) error {
	v.wg.Done()
	return nil
}
