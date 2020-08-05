// +build linux darwin

package kv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoltDB_New(t *testing.T) {
	db, err := New("")
	require.Nil(t, db)
	require.EqualError(t, err, "failed to open db: open : no such file or directory")
}
