package http

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestHTTP_Listen(t *testing.T) {
	proxy := NewHTTP("127.0.0.1:2010")
	go proxy.Listen()
	time.Sleep(200 * time.Millisecond)

	defer proxy.Stop()

	proxy.RegisterHandler("/fake", fakeHandler)

	res, err := http.Get("http://127.0.0.1:2010/fake")
	require.NoError(t, err)

	output, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "hello", string(output))
}

func TestHTPP_Listen_EmptyAddr(t *testing.T) {
	// in this case it will use a random free port
	proxy := NewHTTP("")

	require.Nil(t, proxy.ln)

	go proxy.Listen()
	time.Sleep(200 * time.Millisecond)

	require.NotNil(t, proxy.ln)

	proxy.Stop()
}

func TestHTPP_Listen_BadAddr(t *testing.T) {
	proxy := NewHTTP("bad://xx")

	out := new(bytes.Buffer)
	proxy.logger = zerolog.New(zerolog.ConsoleWriter{Out: out})

	go func() {
		defer func() {
			res := recover()
			require.Regexp(t, "^failed to create conn 'bad://xx':", res)
		}()

		proxy.Listen()
	}()
	time.Sleep(200 * time.Millisecond)

	defer proxy.Stop()

	require.Regexp(t, "failed to create conn 'bad://xx':", out.String())
}

func TestGetAddr(t *testing.T) {
	proxy := NewHTTP("127.0.0.1:2010")
	go proxy.Listen()
	time.Sleep(200 * time.Millisecond)

	defer proxy.Stop()

	addr := proxy.GetAddr()
	require.Equal(t, "127.0.0.1:2010", addr.String())
}

func fakeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello"))
}
