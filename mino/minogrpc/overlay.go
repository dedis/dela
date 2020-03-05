package minogrpc

import (
	context "context"

	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

// gRPC service for the overlay. The handler map points to the one in
// Server.Handlers, which is updated each time the makeRPC function is called.
type overlayService struct {
	handlers map[string]mino.Handler
}

// Call is the implementation of the overlay.Call proto definition
func (o overlayService) Call(ctx context.Context, msg *CallMsg) (*CallResp, error) {
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
		return nil, xerrors.Errorf("failed to unmarshal to any: %v", err)
	}

	result, err := handler.Process(dynamicAny.Message)
	if err != nil {
		return nil, xerrors.Errorf("failed to call the Process function from "+
			"the handler using the provided message: %v", err)
	}

	anyResult, err := ptypes.MarshalAny(result)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal result to any: %v", err)
	}

	return &CallResp{Message: anyResult}, nil
}
