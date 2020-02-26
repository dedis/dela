package m

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var logout = zerolog.ConsoleWriter{
	Out:        os.Stdout,
	TimeFormat: time.RFC3339,
}

// Logger is a globally available logger instance.
var Logger = zerolog.New(logout).
	With().Timestamp().Logger().
	With().Caller().Logger().
	Level(zerolog.DebugLevel)
