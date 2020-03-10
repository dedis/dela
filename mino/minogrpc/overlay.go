package minogrpc

import (
	context "context"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

// gRPC service for the overlay. The handler map points to the one in
// Server.Handlers, which is updated each time the makeRPC function is called.
type overlayService struct {
	handlers map[string]mino.Handler
	// TODO: populate
	addr *mino.Address
}

// Call is the implementation of the overlay.Call proto definition
func (o overlayService) Call(ctx context.Context, msg *OverlayMsg) (*OverlayMsg, error) {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers["apiuri"]
	if !ok {
		return nil, xerrors.Errorf("apiuri not found in context header: ", apiURI)
	}
	if len(apiURI) != 1 {
		return nil, xerrors.Errorf("unexpected number of elements in apiuri "+
			"header. Expected 1, found %d", len(apiURI))
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return nil, xerrors.Errorf("didn't find the '%s' handler in the map "+
			"of handlers, did you register it?", apiURI)
	}

	var dynamicAny ptypes.DynamicAny
	err := ptypes.UnmarshalAny(msg.Message, &dynamicAny)
	if err != nil {
		return nil, encoding.NewAnyDecodingError(msg.Message, err)
	}

	result, err := handler.Process(dynamicAny.Message)
	if err != nil {
		return nil, xerrors.Errorf("failed to call the Process function from "+
			"the handler using the provided message: %v", err)
	}

	anyResult, err := ptypes.MarshalAny(result)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(result, err)
	}

	return &OverlayMsg{Message: anyResult}, nil
}

// Stream ...
func (o overlayService) Stream(stream Overlay_StreamServer) error {
	fmt.Println("inside the overlay Stream")
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	ctx := stream.Context()
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers["apiuri"]
	if !ok {
		return xerrors.Errorf("apiuri not found in context header: ", apiURI)
	}
	if len(apiURI) != 1 {
		return xerrors.Errorf("unexpected number of elements in apiuri "+
			"header. Expected 1, found %d", len(apiURI))
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return xerrors.Errorf("didn't find the '%s' handler in the map "+
			"of handlers, did you register it?", apiURI)
	}

	addrs, ok := headers["addr"]
	if !ok {
		return xerrors.Errorf("addr not found in context header: ", apiURI)
	}
	if len(addrs) != 1 {
		return xerrors.Errorf("unexpected number of elements in addr "+
			"header. Expected 1, found %d", len(addrs))
	}

	addr := addrs[0]
	fmt.Println("histoire d'être fixé: ", addrs)

	sender := sender{
		// empty now
		addr: o.addr,
		participants: []player{
			player{
				addr: &mino.Address{
					// empty for the moment
					Id: addr,
				},
				streamClient: stream,
			},
		},
	}
	receiver := receiver{
		in:   make(chan *OverlayMsg),
		errs: make(chan error),
	}
	go func() {
		msg, err := stream.Recv()
		if err != nil {
			fabric.Logger.Error().Msgf("failed to receive in overlay: %v", err)
			receiver.errs <- xerrors.Errorf("failed to receive in overlay: %v", err)
		}
		receiver.in <- msg
	}()

	fmt.Println("calling handler.Stream")
	err := handler.Stream(sender, receiver)
	if err != nil {
		return xerrors.Errorf("failed to call the stream handler: %v", err)
	}

	fmt.Println("exiting overlay")

	return nil

}
