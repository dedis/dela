package minogrpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric"
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

	// Now I provide metadata but without the required "apiuri" element
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

type testServerStream struct {
	overlayStreamServer
	ctx       context.Context
	recvError bool
}

// This is to mock grpc.ServerStream.Context()
func (t testServerStream) Context() context.Context {
	return t.ctx
}

// This is to mock Overlay_StreamServer.Recv()
func (t testServerStream) Recv() (*OverlayMsg, error) {
	if t.recvError {
		return nil, errors.New("oops from the server")
	}
	anyMsg, err := ptypes.MarshalAny(&empty.Empty{})
	if err != nil {
		fabric.Logger.Fatal().Msg("unexpected nil in marshal: " + err.Error())
	}
	return &OverlayMsg{Message: anyMsg}, nil
}

func Test_Stream(t *testing.T) {
	// ctx := context.Background()
	// msg := &OverlayMsg{}

	overlayService := overlayService{
		handlers: make(map[string]mino.Handler),
	}

	streamServer := testServerStream{
		ctx: context.Background(),
	}

	// The context has no metadata, which should yield an error
	err := overlayService.Stream(&streamServer)
	require.EqualError(t, err, "header not found in provided context")

	// Now I provide metadata but without the required "apiuri" element
	header := metadata.New(map[string]string{"a": "b"})
	streamServer.ctx = metadata.NewIncomingContext(context.Background(), header)
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, fmt.Sprintf("%s not found in context header", headerURIKey))

	// Now I provide metadata but with more than one element at the 'apiuri' key
	header = metadata.New(map[string]string{})
	header.Append(headerURIKey, "a", "b")
	streamServer.ctx = metadata.NewIncomingContext(context.Background(), header)
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, fmt.Sprintf("unexpected number of elements in %s "+
		"header. Expected 1, found %d", headerURIKey, 2))

	// Now with the correct header, but it shouldn't find the handler
	header = metadata.New(map[string]string{})
	header.Append(headerURIKey, "handler_key")
	streamServer.ctx = metadata.NewIncomingContext(context.Background(), header)
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, fmt.Sprintf("didn't find the '%s' handler in the map "+
		"of handlers, did you register it?", "handler_key"))

	// Now I provide a handler but then we miss the address in the header
	// metadata
	overlayService.handlers["handler_key"] = testFailHandler{}
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, fmt.Sprintf("%s not found in context header", headerAddressKey))

	// Now I add more than one element at the address key in the header metadata
	header = metadata.New(map[string]string{})
	header.Append(headerURIKey, "handler_key")
	header.Append(headerAddressKey, "a", "b")
	streamServer.ctx = metadata.NewIncomingContext(context.Background(), header)
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, fmt.Sprintf("unexpected number of elements in %s "+
		"header. Expected 1, found %d", headerAddressKey, 2))

	// Now I set the right elements in the header but use a handler that should
	// raise an error
	header = metadata.New(map[string]string{})
	header.Append(headerURIKey, "handler_key")
	header.Append(headerAddressKey, "")
	streamServer.ctx = metadata.NewIncomingContext(context.Background(), header)
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, "failed to call the stream handler: oops")

	// Now we set our mock StreamServer to return an error on receive
	streamServer.recvError = true
	err = overlayService.Stream(&streamServer)
	require.EqualError(t, err, "failed to call the stream handler: oops")
	// We have to wait there so we catch the goroutine error
	time.Sleep(time.Microsecond * 400)

	// Now we use a handler that checks if an error is received. There should be
	// an error because the receiver.Recv() expects an enveloppe but we are
	// giving an empty
	streamServer.recvError = false
	overlayService.handlers["handler_key"] = testFailHandler2{t: t}
	err = overlayService.Stream(&streamServer)
	require.NoError(t, err)
}

// -------
// Utility functions

// implements a handler interface that returns an error in call and stream
type testFailHandler struct {
	mino.UnsupportedHandler
}

func (t testFailHandler) Process(req proto.Message) (proto.Message, error) {
	return nil, errors.New("oops")
}

func (t testFailHandler) Stream(out mino.Sender, in mino.Receiver) error {
	return errors.New("oops")
}

// implements a handler interface that just returns a wrong message in call and
// checks for an error in stream
type testFailHandler2 struct {
	mino.UnsupportedHandler
	t *testing.T
}

func (t testFailHandler2) Process(req proto.Message) (proto.Message, error) {
	return nil, nil
}

func (t testFailHandler2) Stream(out mino.Sender, in mino.Receiver) error {
	any, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t.t, err)

	_, _, err = in.Recv(context.Background())
	require.EqualError(t.t, err, encoding.NewAnyDecodingError(any, errors.New("mismatched message type: got \"google.protobuf.Empty\" want \"minogrpc.Envelope\"")).Error())
	return nil
}
