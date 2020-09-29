package http

import (
	"io/ioutil"
	"net/http"
	"testing"
	"time"

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

func fakeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello"))
}
