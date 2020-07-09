package pow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution/bmexec"
	txn "go.dedis.ch/dela/core/tap/anon"
	pool "go.dedis.ch/dela/core/tap/pool/mem"
	validation "go.dedis.ch/dela/core/validation/simple"
)

func TestService_Basic(t *testing.T) {
	pool := pool.NewPool()
	srvc := NewService(pool, validation.NewService(bmexec.NewExecution()))

	// 1. Start the ordering service.
	require.NoError(t, srvc.Listen())

	// 2. Watch for new events before sending a transaction.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := srvc.Watch(ctx)

	// 3. Send a transaction to the pool. It should be detected by the ordering
	// service and start a new block.
	require.NoError(t, pool.Add(txn.NewTransaction()))

	evt := <-evts
	require.Equal(t, 1, evt.Index)
}
