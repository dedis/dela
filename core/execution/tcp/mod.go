package tcp

import (
	"encoding/binary"
	"net"
	"os"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

const defaultUnikernelAddr = "192.168.232.128:12345"

var storeKey = [32]byte{0, 0, 10}

const addrArg = "tcp:addr"
const dialTimeout = time.Second * 1

// Service ...
type Service struct {
}

// NewExecution ...
func NewExecution() *Service {
	return &Service{}
}

// Execute ...
func (hs *Service) Execute(snap store.Snapshot, step execution.Step) (execution.Result, error) {
	res := execution.Result{}

	current, err := snap.Get(storeKey[:])
	if err != nil {
		return res, xerrors.Errorf("failed to get store value: %v", err)
	}

	if len(current) == 0 {
		current = make([]byte, 8)
	}

	addr := os.Getenv("UNIKERNEL_TCP")
	if addr == "" {
		addr = defaultUnikernelAddr
	}

	conn, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return res, xerrors.Errorf("failed to connect to tcp with %s: %v", addr, err)
	}

	dela.Logger.Info().Msgf("sending value: %d", binary.LittleEndian.Uint64(current))

	_, err = conn.Write(current)
	if err != nil {
		return res, xerrors.Errorf("failed to send to unikernel: %v", err)
	}

	readRes := make([]byte, 8)

	conn.SetReadDeadline(time.Now().Add(time.Second * 10))

	_, err = conn.Read(readRes)
	if err != nil {
		return res, xerrors.Errorf("failed to read result: %v", err)
	}

	err = snap.Set([]byte(storeKey[:]), readRes)
	if err != nil {
		return res, xerrors.Errorf("failed to set store value: %v", err)
	}

	dela.Logger.Info().Msgf("set new value:  %d", binary.LittleEndian.Uint64(readRes))

	return execution.Result{
		Accepted: true,
	}, nil
}
