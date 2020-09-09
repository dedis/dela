package traffic

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"google.golang.org/grpc/metadata"
)

func TestTraffic_Integration(t *testing.T) {
	traffic := NewTraffic(fake.NewAddress(0), fake.AddressFactory{}, ioutil.Discard)

	a1 := fake.NewAddress(0)
	a2 := fake.NewAddress(1)
	a3 := fake.NewAddress(2)

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
--- from: fake.Address[0]
--- to: fake.Address[1]
--- msg: (type <nil>) %!s(<nil>)
--- context: test
-- item:
--- typeStr: send
--- from: fake.Address[2]
--- to: fake.Address[0]
--- msg: (type traffic.fakePacket) fakePacket
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

// -----------------------------------------------------------------------------
// Utility functions

type fakePacket struct {
	router.Packet
	source mino.Address
	dest   []mino.Address
}

func newFakePacket(source mino.Address, dest ...mino.Address) router.Packet {
	return fakePacket{
		source: source,
		dest:   dest,
	}
}

func (p fakePacket) GetSource() mino.Address {
	return p.source
}

func (p fakePacket) GetDestination() []mino.Address {
	return p.dest
}

func (p fakePacket) String() string {
	return "fakePacket"
}
