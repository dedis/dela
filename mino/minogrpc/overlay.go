package minogrpc

import (
	context "context"

	"github.com/golang/protobuf/ptypes"
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
		return nil, xerrors.Errorf("failed to get the apiuri in context header: ", apiURI)
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return nil, xerrors.Errorf("didn't find '%s' handler, did you "+
			"register it?", apiURI)
	}

	var parsedMsg ptypes.DynamicAny
	err := ptypes.UnmarshalAny(msg.Message, &parsedMsg)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal any: %v", err)
	}

	result, err := handler.Process(parsedMsg.Message)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal message: %v", err)
	}

	anyResult, err := ptypes.MarshalAny(result)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal result to any: %v", err)
	}

	return &CallResp{Message: anyResult}, nil
}
