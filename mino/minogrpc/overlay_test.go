package minogrpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"google.golang.org/grpc/metadata"
)

func Test_Call(t *testing.T) {
	ctx := context.Background()
	msg := &OverlayMsg{}

	overlayService := overlayService{
		handlers: make(map[string]mino.Handler),
	}

	// The context has no metadata, which should yield an error
	_, err := overlayService.Call(ctx, msg)
	require.EqualError(t, err, "header not found in provided context")

	// Now I provide metadata but without the require "apiuri" element
	header := metadata.New(map[string]string{"a": "b"})
	ctx = metadata.NewIncomingContext(context.Background(), header)
	_, err = overlayService.Call(ctx, msg)
	require.EqualError(t, err, fmt.Sprintf("%s not found in context header", headerURIKey))

	// Now I provide metadata but with more than one element at the 'apiuri' key
	header = metadata.New(map[string]string{})
	header.Append(headerURIKey, "a", "b")
	ctx = metadata.NewIncomingContext(context.Background(), header)
	_, err = overlayService.Call(ctx, msg)
	require.EqualError(t, err, fmt.Sprintf("unexpected number of elements in %s "+
		"header. Expected 1, found %d", headerURIKey, 2))

	// Now with the correct header, but it shouldn't find the handler
	header = metadata.New(map[string]string{})
	header.Append(headerURIKey, "handler_key")
	ctx = metadata.NewIncomingContext(context.Background(), header)
	_, err = overlayService.Call(ctx, msg)
	require.EqualError(t, err, fmt.Sprintf("didn't find the '%s' handler in the map "+
		"of handlers, did you register it?", "handler_key"))

	// Now I provide a handler but leave 'msg.Message' to nil, which should
	// yield a decoding error
	overlayService.handlers["handler_key"] = testFailHandler{}
	_, err = overlayService.Call(ctx, msg)
	require.EqualError(t, err, encoding.NewAnyDecodingError(msg.Message, errors.New("message is nil")).Error())

	// Now set the 'msg.Message', but the handler should retrun an error
	anyMsg, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	msg.Message = anyMsg
	_, err = overlayService.Call(ctx, msg)
	require.EqualError(t, err, "failed to call the Process function from the handler using the provided message: oops")

	// Now use a handler that returns a wrong message
	overlayService.handlers["handler_key"] = testFailHandler2{}
	_, err = overlayService.Call(ctx, msg)
	require.EqualError(t, err, encoding.NewAnyEncodingError(nil, errors.New("proto: Marshal called with nil")).Error())
}

// -------
// Utility functions

// implements a handler interface that returns an error in call
type testFailHandler struct {
	mino.UnsupportedHandler
}

func (t testFailHandler) Process(req proto.Message) (proto.Message, error) {
	return nil, errors.New("oops")
}

// implements a handler interface that just returns a wrong message in call
type testFailHandler2 struct {
	mino.UnsupportedHandler
}

func (t testFailHandler2) Process(req proto.Message) (proto.Message, error) {
	return nil, nil
}
