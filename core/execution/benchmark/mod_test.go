package main

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/evm"

	"go.dedis.ch/dela/core/execution/tcp"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/kyber/v3/suites"
)

const iterations = 50

var suite = suites.MustFind("Ed25519")

func BenchmarkLocal(b *testing.B) {
	testWithAddr(b, "127.0.0.1:12346")
}

func BenchmarkUnikernel(b *testing.B) {
	testWithAddr(b, "192.168.232.128:12345")
}

func BenchmarkEVMIncrement(b *testing.B) {
	n := iterations

	storage := newInmemory()
	step := execution.Step{Previous: []txn.Transaction{}, Current: tx{
		args: map[string][]byte{"contractName": []byte("increment")},
	}}

	exec, err := evm.NewExecution("increment")
	require.NoError(b, err)

	for i := 0; i < n; i++ {
		_, err := exec.Execute(storage, step)
		if err != nil {
			b.Logf("failed to execute: %+v", err)
			b.FailNow()
		}
	}
}

func BenchmarkEVMNetwork(b *testing.B) {
	testWithAddr(b, "127.0.0.1:12347")
}

func BenchmarkNativeEC(b *testing.B) {
	for i := 0; i < iterations; i++ {
		scalar := suite.Scalar().Pick(suite.RandomStream())
		_, err := scalar.MarshalBinary()
		require.NoError(b, err)

		point := suite.Point().Mul(scalar, nil)
		_, err = point.MarshalBinary()
		require.NoError(b, err)
	}
}

func BenchmarkEVMEC(b *testing.B) {
	storage := newInmemory()
	step := execution.Step{Previous: []txn.Transaction{}, Current: tx{
		args: map[string][]byte{"contractName": []byte("Ed25519")},
	}}

	exec, err := evm.NewExecution("Ed25519")
	require.NoError(b, err)

	storeKey := [32]byte{0, 0, 10}
	gasUsageKey := [32]byte{0, 0, 20}
	runCountKey := [32]byte{0, 0, 30}
	//resultKey := [32]byte{0, 0, 40}

	storage.Set(gasUsageKey[:], make([]byte, 8))
	storage.Set(runCountKey[:], make([]byte, 8))

	for i := 0; i < iterations; i++ {
		scalar := suite.Scalar().Pick(suite.RandomStream())

		scalarBuf, err := scalar.MarshalBinary()
		require.NoError(b, err)

		storage.Set(storeKey[:], scalarBuf)
		_, err = exec.Execute(storage, step)
		if err != nil {
			b.Logf("failed to execute: %+v", err)
			b.FailNow()
		}
	}

	gasUsageBuf, err := storage.Get(gasUsageKey[:])
	require.NoError(b, err)

	gasUsage := float64(binary.LittleEndian.Uint64(gasUsageBuf))

	runCountBuf, err := storage.Get(runCountKey[:])
	require.NoError(b, err)

	runCount := float64(binary.LittleEndian.Uint64(runCountBuf))

	fmt.Printf("Did %f multiplications. Average Gas Usage=%.2f\n", runCount, gasUsage/runCount)

}

func testWithAddr(b *testing.B, addr string) {
	n := iterations

	storage := newInmemory()
	step := execution.Step{Previous: []txn.Transaction{}, Current: tx{
		args: map[string][]byte{"tcp:addr": []byte(addr)},
	}}
	exec := tcp.NewExecution()

	for i := 0; i < n; i++ {
		_, err := exec.Execute(storage, step)
		if err != nil {
			b.Logf("failed to execute; %v", err)
			b.FailNow()
		}
	}
}

type inmemory struct {
	store.Readable
	store.Writable

	data map[string][]byte
}

func newInmemory() inmemory {
	return inmemory{
		data: make(map[string][]byte),
	}
}

func (i inmemory) Get(key []byte) ([]byte, error) {
	return i.data[string(key)], nil
}

func (i inmemory) Set(key []byte, value []byte) error {
	i.data[string(key)] = value
	return nil
}

func (i inmemory) Delete(key []byte) error {
	delete(i.data, string(key))
	return nil
}

type tx struct {
	txn.Transaction
	args map[string][]byte
}

func (t tx) GetArg(key string) []byte {
	return t.args[key]
}
