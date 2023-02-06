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
	time.Sleep(1 * time.Second)
	isAfter := stats.insertionTime.After(now)
	require.True(t, isAfter)
}
