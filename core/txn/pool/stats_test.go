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

	isRotten := time.Since(stats.insertionTime) > time.Minute
	require.True(t, isRotten)
	stats.ResetStats()
	isRotten = time.Since(stats.insertionTime) > time.Minute
	require.False(t, isRotten)
}
