package fake

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// WaitLog is a helper to wait for a log to be printed. It executes the callback
// when it detects it.
func WaitLog(msg string, t time.Duration) (zerolog.Logger, func(t *testing.T)) {
	reader, writer := io.Pipe()
	done := make(chan struct{})
	found := false

	buffer := new(bytes.Buffer)
	tee := io.TeeReader(reader, buffer)

	go func() {
		select {
		case <-done:
		case <-time.After(t):
			writer.Close()
		}
	}()

	go func() {
		defer close(done)

		data := make([]byte, 1024)

		for {
			n, err := tee.Read(data)
			if err != nil {
				return
			}

			if strings.Contains(string(data[:n]), fmt.Sprintf(`"%s"`, msg)) {
				found = true
				return
			}
		}
	}()

	wait := func(t *testing.T) {
		<-done
		if !found {
			t.Fatalf("log not found in %s", buffer.String())
		}
	}

	return zerolog.New(writer), wait
}

// CheckLog returns a logger and a check function. When called, the function
// will verify if the logger has seen the message printed.
func CheckLog(msg string) (zerolog.Logger, func(t *testing.T)) {
	buffer := new(bytes.Buffer)

	check := func(t *testing.T) {
		require.Contains(t, buffer.String(), fmt.Sprintf(`"%s"`, msg))
	}

	return zerolog.New(buffer), check
}
