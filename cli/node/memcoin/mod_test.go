package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemcoin_Main(t *testing.T) {
	main()
}

func TestMemcoin_Scenario_1(t *testing.T) {
	sigs := make(chan os.Signal)
	wg := sync.WaitGroup{}
	wg.Add(3)

	node1 := filepath.Join(os.TempDir(), "memcoin", "node1")
	node2 := filepath.Join(os.TempDir(), "memcoin", "node2")
	node3 := filepath.Join(os.TempDir(), "memcoin", "node3")

	cfg := config{Channel: sigs, Writer: ioutil.Discard}

	// Run node 1
	go func() {
		defer wg.Done()

		err := runWithCfg(makeNodeArg(node1, 2111), cfg)
		require.NoError(t, err)
	}()

	// Run node 2
	go func() {
		defer wg.Done()

		err := runWithCfg(makeNodeArg(node2, 2112), cfg)
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()

		err := runWithCfg(makeNodeArg(node3, 2113), cfg)
		require.NoError(t, err)
	}()

	defer func() {
		// Simulate a Ctrl+C
		close(sigs)
		wg.Wait()

		os.RemoveAll(node1)
		os.RemoveAll(node2)
		os.RemoveAll(node3)
	}()

	time.Sleep(500 * time.Millisecond)

	// Share the certificates.
	shareCert(t, node2, node1)
	shareCert(t, node3, node1)

	// Setup the chain with nodes 1 and 2.
	args := append(
		append(
			[]string{os.Args[0], "--config", node1, "ordering", "setup"},
			getExport(t, node1)...,
		),
		getExport(t, node2)...)

	err := run(args)
	require.NoError(t, err)

	// Add node 3 to the current chain.
	args = append([]string{os.Args[0], "--config", node1, "ordering", "roster", "add"}, getExport(t, node3)...)

	err = run(args)
	require.NoError(t, err)

	// Test a bad command.
	err = runWithCfg([]string{os.Args[0], "ordering", "setup"}, cfg)
	require.EqualError(t, err, `Required flag "member" not set`)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeNodeArg(path string, port uint16) []string {
	return []string{
		os.Args[0], "--config", path, "start", "--port", strconv.Itoa(int(port)),
	}
}

func shareCert(t *testing.T, path string, src string) {
	args := append(
		[]string{os.Args[0], "--config", path, "minogrpc", "join", "--address", "127.0.0.1:2111"},
		getToken(t, src)...,
	)

	err := run(args)
	require.NoError(t, err)
}

func getToken(t *testing.T, path string) []string {
	buffer := new(bytes.Buffer)
	cfg := config{
		Writer: buffer,
	}

	args := []string{os.Args[0], "--config", path, "minogrpc", "token"}
	err := runWithCfg(args, cfg)
	require.NoError(t, err)

	return strings.Split(buffer.String(), " ")
}

func getExport(t *testing.T, path string) []string {
	buffer := bytes.NewBufferString("--member ")
	cfg := config{
		Writer: buffer,
	}

	args := []string{os.Args[0], "--config", path, "ordering", "export"}

	err := runWithCfg(args, cfg)
	require.NoError(t, err)

	return strings.Split(buffer.String(), " ")
}
