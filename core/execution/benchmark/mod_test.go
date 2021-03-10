package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/evm"
	"go.dedis.ch/dela/core/execution/tcp"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
)

const iterations = 50

func BenchmarkLocal(b *testing.B) {
	testWithAddr(b, "127.0.0.1:12346")
}

func BenchmarkUnikernel(b *testing.B) {
	testWithAddr(b, "192.168.232.128:12345")
}

func BenchmarkEVM(b *testing.B) {
	n := iterations

	storage := newInmemory()
	step := execution.Step{Previous: []txn.Transaction{}, Current: tx{}}

	exec, err := evm.NewExecution()
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
