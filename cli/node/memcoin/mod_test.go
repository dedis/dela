package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
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
	dir, err := ioutil.TempDir(os.TempDir(), "memcoin1")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	sigs := make(chan os.Signal)
	wg := sync.WaitGroup{}
	wg.Add(5)

	node1 := filepath.Join(dir, "node1")
	node2 := filepath.Join(dir, "node2")
	node3 := filepath.Join(dir, "node3")
	node4 := filepath.Join(dir, "node4")
	node5 := filepath.Join(dir, "node5")

	cfg := config{Channel: sigs, Writer: ioutil.Discard}

	runNode(t, node1, cfg, 2111, &wg)
	runNode(t, node2, cfg, 2112, &wg)
	runNode(t, node3, cfg, 2113, &wg)
	runNode(t, node4, cfg, 2114, &wg)
	runNode(t, node5, cfg, 2115, &wg)

	defer func() {
		// Simulate a Ctrl+C
		close(sigs)
		wg.Wait()
	}()

	require.True(t, waitDaemon(t, []string{node1, node2, node3}), "timeout")

	// Share the certificates.
	shareCert(t, node2, node1, "127.0.0.1:2111")
	shareCert(t, node3, node1, "127.0.0.1:2111")
	shareCert(t, node5, node1, "127.0.0.1:2111")

	// Setup the chain with nodes 1 and 2.
	args := append(append(
		append(
			[]string{os.Args[0], "--config", node1, "ordering", "setup"},
			getExport(t, node1)...,
		),
		getExport(t, node2)...),
		getExport(t, node3)...)

	err = run(args)
	require.NoError(t, err)

	// Add node 4 to the current chain. This node is not reachable from the
	// others but transactions should work as the threshold is correct.
	args = append([]string{
		os.Args[0],
		"--config", node1, "ordering", "roster", "add",
		"--wait", "60s"},
		getExport(t, node4)...,
	)

	err = run(args)
	require.NoError(t, err)

	// Add node 5 which should be participating.
	args = append([]string{
		os.Args[0],
		"--config", node1, "ordering", "roster", "add",
		"--wait", "60s"},
		getExport(t, node5)...,
	)

	err = run(args)
	require.NoError(t, err)

	buffer := new(bytes.Buffer)

	// Run a few transactions.
	for i := 0; i < 5; i++ {
		buffer.Reset()
		err = runWithCfg(args, config{Writer: buffer})
		require.NoError(t, err)
		require.Contains(t, buffer.String(), "transaction refused: duplicate in roster")
	}

	// Test a timeout waiting for a transaction.
	args[7] = "1ns"
	buffer.Reset()
	err = runWithCfg(args, config{Writer: buffer})
	require.NoError(t, err)
	require.Contains(t, buffer.String(), "transaction not found after timeout\n")

	// Test a bad command.
	err = runWithCfg([]string{os.Args[0], "ordering", "setup"}, cfg)
	require.EqualError(t, err, `Required flag "member" not set`)
}

func TestMemcoin_Scenario_2(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "memcoin2")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	node1 := filepath.Join(dir, "node1")
	node2 := filepath.Join(dir, "node2")

	// Setup the chain and closes the node.
	setupChain(t, []string{node1, node2}, []uint16{2210, 2211})

	sigs := make(chan os.Signal)
	wg := sync.WaitGroup{}
	wg.Add(2)

	cfg := config{Channel: sigs, Writer: ioutil.Discard}

	runNode(t, node1, cfg, 2210, &wg)
	runNode(t, node2, cfg, 2211, &wg)

	defer func() {
		// Simulate a Ctrl+C
		close(sigs)
		wg.Wait()
	}()

	require.True(t, waitDaemon(t, []string{node1, node2}), "timeout")

	args := append([]string{
		os.Args[0],
		"--config", node1, "ordering", "roster", "add",
		"--wait", "60s"},
		getExport(t, node1)...,
	)

	err = run(args)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func runNode(t *testing.T, node string, cfg config, port uint16, wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()

		err := runWithCfg(makeNodeArg(node, port), cfg)
		require.NoError(t, err)
	}()
}

func setupChain(t *testing.T, nodes []string, ports []uint16) {
	sigs := make(chan os.Signal)
	wg := sync.WaitGroup{}
	wg.Add(len(nodes))

	cfg := config{Channel: sigs, Writer: ioutil.Discard}

	for i, node := range nodes {
		runNode(t, node, cfg, ports[i], &wg)
	}

	defer func() {
		// Simulate a Ctrl+C
		close(sigs)
		wg.Wait()
	}()

	waitDaemon(t, nodes)

	shareCert(t, nodes[1], nodes[0], fmt.Sprintf("127.0.0.1:%d", ports[0]))

	// Setup the chain with nodes 1.
	args := append(append(
		[]string{os.Args[0], "--config", nodes[0], "ordering", "setup"},
		getExport(t, nodes[0])...),
		getExport(t, nodes[1])...,
	)

	err := run(args)
	require.NoError(t, err)
}

func waitDaemon(t *testing.T, daemons []string) bool {
	num := 50

	for _, daemon := range daemons {
		for i := 0; i < num; i++ {
			// Windows: we have to check the file as Dial on Windows creates the
			// file and prevent to listen.
			_, err := os.Stat(filepath.Join(daemon, "daemon.sock"))
			if !os.IsNotExist(err) {
				conn, err := net.Dial("unix", filepath.Join(daemon, "daemon.sock"))
				if err == nil {
					conn.Close()
					break
				}
			}

			time.Sleep(100 * time.Millisecond)

			if i+1 >= num {
				return false
			}
		}
	}

	return true
}

func makeNodeArg(path string, port uint16) []string {
	return []string{
		os.Args[0], "--config", path, "start", "--port", strconv.Itoa(int(port)),
	}
}

func shareCert(t *testing.T, path string, src string, addr string) {
	args := append(
		[]string{os.Args[0], "--config", path, "minogrpc", "join", "--address", addr},
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
