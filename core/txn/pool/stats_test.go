package pool

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStats_Reset(t *testing.T) {
	stats := transactionStats{
		insertionTime: time.Now().Add(-time.Hour),
	}

	now := time.Now()
	require.False(t, stats.insertionTime.After(now))

	stats.ResetStats()
	time.Sleep(10 * time.Millisecond)
	require.True(t, stats.insertionTime.After(now))
}
