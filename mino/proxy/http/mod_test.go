package http

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	os.Setenv("PROXY_LOG", "warn")
	setLogLevel()
	require.Equal(t, defaultLevel, zerolog.WarnLevel)

	os.Setenv("PROXY_LOG", "no")
	setLogLevel()
	require.Equal(t, defaultLevel, zerolog.Disabled)

	os.Setenv("PROXY_LOG", "info")
	setLogLevel()
	require.Equal(t, defaultLevel, zerolog.InfoLevel)
}

func TestHTTP_Listen(t *testing.T) {
	proxy := NewHTTP("127.0.0.1:2010")
	go proxy.Listen()
	time.Sleep(200 * time.Millisecond)

	defer proxy.Stop()

	proxy.RegisterHandler("/fake", fakeHandler)

	res, err := http.Get("http://127.0.0.1:2010/fake")
	require.NoError(t, err)

	output, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "hello", string(output))
}

func TestHTPP_Listen_EmptyAddr(t *testing.T) {
	// in this case it will use a random free port
	proxy := NewHTTP("")
	httpProxy := proxy.(*HTTP)

	require.Nil(t, httpProxy.ln)

	go proxy.Listen()
	time.Sleep(200 * time.Millisecond)

	require.NotNil(t, httpProxy.ln)

	proxy.Stop()
}

func TestHTPP_Listen_BadAddr(t *testing.T) {
	proxy := NewHTTP("bad://xx")
	httpProxy := proxy.(*HTTP)

	out := new(bytes.Buffer)
	httpProxy.logger = zerolog.New(zerolog.ConsoleWriter{Out: out})

	go func() {
		defer func() {
			res := recover()
			require.Regexp(t, "^failed to create conn 'bad://xx':", res)
		}()

		proxy.Listen()
	}()
	time.Sleep(400 * time.Millisecond)

	defer proxy.Stop()

	require.Regexp(t, "failed to create conn 'bad://xx':", out.String())
}

func TestGetAddr_Happy(t *testing.T) {
	proxy := NewHTTP("127.0.0.1:2010")
	go proxy.Listen()
	time.Sleep(200 * time.Millisecond)

	defer proxy.Stop()

	addr := proxy.GetAddr()
	require.Equal(t, "127.0.0.1:2010", addr.String())
}

func TestGetAddr_Nil(t *testing.T) {
	proxy := NewHTTP("127.0.0.1:2010")
	require.Nil(t, proxy.GetAddr())
}

func fakeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello"))
}
