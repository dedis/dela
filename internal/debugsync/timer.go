package debugsync

import (
	"go.dedis.ch/dela"
	"runtime/debug"
	"time"
)

const mutexTimeout = 60 * time.Second

func startLockTimer(msg string) chan struct{} {
	done := make(chan struct{})

	stack := debug.Stack()
	go func() {
		select {
		case <-time.After(mutexTimeout):
			dela.Logger.Error().Msgf("%v : %v", msg, string(stack))
			return
		case <-done:
			return
		}
	}()

	return done
}
