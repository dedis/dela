package traffic

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"google.golang.org/grpc/metadata"
)

func TestTraffic_Integration(t *testing.T) {
	src := fake.NewAddress(0)
	a2 := fake.NewAddress(1)
	gw := fake.NewAddress(2)

	traffic := NewTraffic(src, io.Discard)

	header := metadata.New(map[string]string{headerURIKey: "test"})
	ctx := metadata.NewOutgoingContext(context.Background(), header)

	traffic.LogRecv(ctx, gw, newFakePacket(src, a2))
	traffic.LogSend(context.Background(), gw, newFakePacket(fake.NewAddress(0)))

	buffer := new(bytes.Buffer)
	traffic.Display(buffer)

	expected := `- traffic:
-- item:
--- typeStr: received
--- node: fake.Address[0]
--- gateway: fake.Address[2]
--- msg: (type traffic.fakePacket) fakePacket
---- To: [fake.Address[1]]
--- context: test
-- item:
--- typeStr: send
--- node: fake.Address[0]
--- gateway: fake.Address[2]
--- msg: (type traffic.fakePacket) fakePacket
---- To: []
--- context: 
`
	require.Equal(t, expected, buffer.String())

	file := filepath.Join(os.TempDir(), "minogrpc-test-traffic")

	err := traffic.Save(file, true, true)
	require.NoError(t, err)

	defer os.Remove(file)

	content, err := os.ReadFile(file)
	require.NoError(t, err)
	require.True(t, len(content) > 0)
}

func TestSaveItems(t *testing.T) {
	file, err := os.CreateTemp(os.TempDir(), "")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	err = SaveItems(file.Name(), true, true)
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		return
	}

	err = SaveItems("/items.dot", true, true)
	require.Contains(t, err.Error(), "file: open /items.dot: ")
}

func TestSaveEvents(t *testing.T) {
	file, err := os.CreateTemp(os.TempDir(), "")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	err = SaveEvents(file.Name())
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		return
	}

	err = SaveEvents("/events.dot")
	require.Contains(t, err.Error(), "file: open /events.dot: ")
}

func TestTraffic_Save(t *testing.T) {
	traffic := NewTraffic(fake.NewAddress(0), io.Discard)

	if runtime.GOOS == "windows" {
		return
	}

	err := traffic.Save("/traffic.dot", false, false)
	require.Contains(t, err.Error(), "file: open /traffic.dot: ")
}

func TestTraffic_LogRecv(t *testing.T) {
	var traffic *Traffic

	// Should not panic
	traffic.LogRecv(context.Background(), nil, nil)

	traffic = NewTraffic(fake.NewAddress(0), io.Discard)
	require.Len(t, traffic.items, 0)

	traffic.LogRecv(context.Background(), nil, nil)
	require.Len(t, traffic.items, 1)

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.New(nil))
	traffic.LogRecv(ctx, nil, nil)
	require.Len(t, traffic.items, 2)
}

func TestTraffic_LogRelay(t *testing.T) {
	var traffic *Traffic
	traffic.LogRelay(fake.NewAddress(1))

	traffic = NewTraffic(fake.NewAddress(0), io.Discard)
	require.Len(t, traffic.events, 0)

	traffic.LogRelay(fake.NewAddress(5))
	require.Len(t, traffic.events, 1)
}

func TestGenerateItemsGraphViz(t *testing.T) {
	traffic := NewTraffic(fake.NewAddress(0), io.Discard)
	traffic.LogRecv(context.Background(), fake.NewAddress(1), newFakePacket(fake.NewAddress(2)))
	traffic.LogSend(context.Background(), fake.NewAddress(1), newFakePacket(fake.NewAddress(2)))

	traffic2 := NewTraffic(fake.NewAddress(1), io.Discard)

	traffic3 := NewTraffic(fake.NewAddress(3), io.Discard)
	traffic3.LogRelay(fake.NewAddress(0))

	buffer := new(bytes.Buffer)
	GenerateItemsGraphviz(buffer, false, false, traffic2, traffic, traffic3)
	require.NotContains(t, buffer.String(), "received")
	require.NotContains(t, buffer.String(), "send")
}

func TestGenerateEventGraphViz(t *testing.T) {
	traffic := NewTraffic(fake.NewAddress(0), io.Discard)
	traffic.LogRelay(fake.NewAddress(1))
	traffic.LogRelay(fake.NewAddress(2))
	traffic.LogRelayClosed(fake.NewAddress(2))

	buffer := new(bytes.Buffer)
	GenerateEventGraphviz(buffer, traffic)
	require.Contains(t, buffer.String(), `"fake.Address[0]" -> "fake.Address[1]"`)
}

func TestWatcherIns(t *testing.T) {
	watcher := GlobalWatcher
	events := watcher.WatchIns(context.Background())

	traffic := NewTraffic(fake.NewAddress(0), io.Discard)

	addr := fake.NewAddress(0)
	pkt := newFakePacket(fake.NewAddress(1), fake.NewAddress(2))
	traffic.LogRecv(context.Background(), addr, pkt)

	select {
	case event := <-events:
		require.Equal(t, addr, event.Address)
		require.Equal(t, pkt, event.Pkt)
	default:
		t.Error("events not saved")
	}
}

func TestWatcherOuts(t *testing.T) {
	watcher := GlobalWatcher
	events := watcher.WatchOuts(context.Background())

	traffic := NewTraffic(fake.NewAddress(0), io.Discard)

	addr := fake.NewAddress(0)
	pkt := newFakePacket(fake.NewAddress(1), fake.NewAddress(2))
	traffic.LogSend(context.Background(), addr, pkt)

	select {
	case event := <-events:
		require.Equal(t, addr, event.Address)
		require.Equal(t, pkt, event.Pkt)
	default:
		t.Error("events not saved")
	}
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
