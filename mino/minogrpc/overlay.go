package minogrpc

import (
	context "context"

	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

// gRPC service for the overlay.
type overlayService struct {
	handlers map[string]mino.Handler
}

// Call is the implementation of the overlay.Call proto definition
func (o overlayService) Call(ctx context.Context, msg *CallMsg) (*CallResp, error) {
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers["apiuri"]
	if !ok || len(apiURI) != 1 {
		return nil, xerrors.Errorf("failed to get the apiURI in context header: ", apiURI)
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return nil, xerrors.Errorf("didn't find '%s' handler, did you "+
			"register it?", apiURI)
	}

	result, err := handler.Process(msg.Message)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal message: %v", err)
	}

	anyResult, ok := result.(*any.Any)
	if !ok {
		return nil, xerrors.Errorf("failed to cast result to any.Any: %v", err)
	}

	return &CallResp{Message: anyResult}, nil
}
