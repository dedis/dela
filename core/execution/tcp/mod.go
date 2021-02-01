package tcp

import (
	"net"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

const unikernelAddr = "192.168.232.128:12345"
const storeKey = "tcp:store"

// Service ...
type Service struct {
	conn net.Conn
}

// NewExecution ...
func NewExecution() *Service {
	conn, err := net.Dial("tcp", unikernelAddr)
	if err != nil {
		dela.Logger.Fatal().Err(err).Msg("failed to connect to unikernel")
	}

	return &Service{
		conn: conn,
	}
}

// Execute ...
func (hs *Service) Execute(snap store.Snapshot, step execution.Step) (execution.Result, error) {
	res := execution.Result{}

	current, err := snap.Get([]byte(storeKey))
	if err != nil {
		return res, xerrors.Errorf("failed to get store value: %v", err)
	}

	if len(current) == 0 {
		current = []byte{0}
	}

	dela.Logger.Info().Msgf("sending value: %d", current)

	_, err = hs.conn.Write(current)
	if err != nil {
		return res, xerrors.Errorf("failed to send to unikernel: %v", err)
	}

	readRes := make([]byte, len(current)+1)

	hs.conn.SetReadDeadline(time.Now().Add(time.Second * 10))

	n, err := hs.conn.Read(readRes)
	if err != nil {
		return res, xerrors.Errorf("failed to read result: %v", err)
	}

	err = snap.Set([]byte(storeKey), readRes[:n])
	if err != nil {
		return res, xerrors.Errorf("failed to set store value: %v", err)
	}

	dela.Logger.Info().Msgf("set new value: %d", readRes[:n])

	return execution.Result{
		Accepted: true,
	}, nil
}
