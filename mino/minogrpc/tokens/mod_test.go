package tokens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInMemoryHolder_Generate(t *testing.T) {
	holder := NewInMemoryHolder()

	token := holder.Generate(time.Minute)
	require.NotEmpty(t, token)
	require.Len(t, holder.tokens, 1)
	require.True(t, time.Now().Before(holder.tokens[token]))
	require.True(t, time.Now().Add(time.Minute+1).After(holder.tokens[token]))

	holder.Generate(time.Millisecond)
	require.Len(t, holder.tokens, 2)
}

func TestInMemoryHolder_Verify(t *testing.T) {
	holder := NewInMemoryHolder()

	holder.tokens["abc"] = time.Now().Add(time.Minute)
	holder.tokens["def"] = time.Now()

	require.True(t, holder.Verify("abc"))
	require.False(t, holder.Verify("abcd"))
	require.False(t, holder.Verify("def"))
}
