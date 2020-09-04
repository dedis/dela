package minogrpc

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"google.golang.org/grpc/metadata"
)

func TestTraffic_Integration(t *testing.T) {
	traffic := newTraffic(address{host: "A"}, AddressFactory{}, ioutil.Discard)

	a1 := address{host: "A"}
	a2 := address{host: "B"}
	a3 := address{host: "C"}

	header := metadata.New(map[string]string{headerURIKey: "test"})
	ctx := metadata.NewOutgoingContext(context.Background(), header)

	// msg := &Message{From: []byte("D")}

	traffic.logRcv(ctx, a1, a2)
	traffic.logSend(context.Background(), a3, a1, newFakePacket(fake.NewAddress(0)))

	buffer := new(bytes.Buffer)
	traffic.Display(buffer)

	expected := `- traffic:
-- item:
--- typeStr: received
--- from: A
--- to: B
--- msg: (type <nil>) %!s(<nil>)
--- context: test
-- item:
--- typeStr: send
--- from: C
--- to: A
--- msg: (type minogrpc.fakePacket) fakePacket
---- To: []
--- context: 
`
	require.Equal(t, expected, buffer.String())

	file := filepath.Join(os.TempDir(), "minogrpc-test-traffic")

	err := traffic.Save(file, true, true)
	require.NoError(t, err)

	defer os.Remove(file)

	content, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	require.True(t, len(content) > 0)
}
