package tcp

import (
	"encoding/binary"
	"net"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

const aunikernelAddr = "192.168.232.128:12345"
const storeKey = "tcp:store"
const addrArg = "tcp:addr"

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

	addr := step.Current.GetArg(addrArg)
	if len(addr) == 0 {
		return res, xerrors.Errorf("%s argument not found", addrArg)
	}

	current, err := snap.Get([]byte(storeKey))
	if err != nil {
		return res, xerrors.Errorf("failed to get store value: %v", err)
	}

	if len(current) == 0 {
		current = make([]byte, 8)
	}

	conn, err := net.Dial("tcp", string(addr))
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

	err = snap.Set([]byte(storeKey), readRes)
	if err != nil {
		return res, xerrors.Errorf("failed to set store value: %v", err)
	}

	dela.Logger.Info().Msgf("set new value:  %d", binary.LittleEndian.Uint64(readRes))

	return execution.Result{
		Accepted: true,
	}, nil
}
