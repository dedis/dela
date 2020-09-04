package access

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Compile(t *testing.T) {
	str := Compile("a", "b", "c")
	require.Equal(t, str, "a:b:c")
}
