package qsc

import (
	"crypto/rand"
	"fmt"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
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

	for j := 0; j < n; j++ {
		c := cons[j]
		actor := actors[j]
		go func() {
			for i := 0; i < k; i++ {
				err := actor.Propose(newFakeProposal(), nil)
				require.NoError(t, err)
			}
			close(c.ch)
		}()
	}

	val.wg.Wait()

	require.Equal(t, cons[0].history, cons[1].history)
	require.Len(t, cons[0].history, k)
	t.Logf("%v\n", cons[0].history)
	t.Logf("%v\n", cons[1].history)
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

type fakeProposal struct {
	consensus.Proposal
	hash []byte
}

func newFakeProposal() fakeProposal {
	buffer := make([]byte, 32)
	rand.Read(buffer)
	return fakeProposal{hash: buffer}
}

func (p fakeProposal) GetHash() []byte {
	return p.hash
}
