package minogrpc

import (
	context "context"
	"crypto/tls"
	"io"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// gRPC service for the overlay. The handler map points to the one in
// Server.Handlers, which is updated each time the makeRPC function is called.
type overlayService struct {
	encoder  encoding.ProtoMarshaler
	handlers map[string]mino.Handler
	// this is the address of the server. This address is used to provide
	// insighful information in the traffic history, as it is used to form the
	// addressID of the sender.
	addr address
	// This map is used to create a new stream connection if possible
	neighbour map[string]Peer
	// This certificate is used to create a new stream connection if possible
	srvCert *tls.Certificate
	// Used to record traffic activity
	traffic        *traffic
	routingFactory RoutingFactory
}

// Call is the implementation of the overlay.Call proto definition
func (o overlayService) Call(ctx context.Context, msg *OverlayMsg) (*OverlayMsg, error) {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers[headerURIKey]
	if !ok {
		return nil, xerrors.Errorf("%s not found in context header", headerURIKey)
	}
	if len(apiURI) != 1 {
		return nil, xerrors.Errorf("unexpected number of elements in %s "+
			"header. Expected 1, found %d", headerURIKey, len(apiURI))
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return nil, xerrors.Errorf("didn't find the '%s' handler in the map "+
			"of handlers, did you register it?", apiURI[0])
	}

	message, err := o.encoder.UnmarshalDynamicAny(msg.Message)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	req := mino.Request{
		Address: o.addr,
		Message: message,
	}

	result, err := handler.Process(req)
	if err != nil {
		return nil, xerrors.Errorf("failed to call the Process function from "+
			"the handler using the provided message: %v", err)
	}

	anyResult, err := o.encoder.MarshalAny(result)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal result: %v", err)
	}

	return &OverlayMsg{Message: anyResult}, nil
}

// Stream is the fonction used to perform mino.RPC.Stream() calls. It is called
// by the client side.
func (o overlayService) Stream(stream Overlay_StreamServer) error {
	// We fetch the uri that identifies the handler in the handlers map with the
	// grpc metadata api. Using context.Value won't work.
	ctx := stream.Context()
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return xerrors.Errorf("header not found in provided context")
	}

	apiURI, ok := headers[headerURIKey]
	if !ok {
		return xerrors.Errorf("%s not found in context header", headerURIKey)
	}
	if len(apiURI) != 1 {
		return xerrors.Errorf("unexpected number of elements in apiuri "+
			"header. Expected 1, found %d", len(apiURI))
	}

	handler, ok := o.handlers[apiURI[0]]
	if !ok {
		return xerrors.Errorf("didn't find the '%s' handler in the map "+
			"of handlers, did you register it?", apiURI[0])
	}

	rpcID := o.addr.String()

	// Listen on the first message, which should be the routing infos
	overlayMsg, err := stream.Recv()
	if err != nil {
		return xerrors.Errorf("failed to receive first routing message: %v", err)
	}

	routingMsg := &RoutingMsg{}
	err = o.encoder.UnmarshalAny(overlayMsg.Message, routingMsg)
	if err != nil {
		return xerrors.Errorf("failed to decode first routing message: %v", err)
	}

	addrs := make([]mino.Address, len(routingMsg.Addrs))
	for i, addrStr := range routingMsg.Addrs {
		addrs[i] = address{addrStr}
	}

	// TODO: use an interface and allow different types of routing
	routing, err := o.routingFactory.FromAddrs(addrs, map[string]interface{}{
		TreeRoutingOpts.Addr: o.addr, "treeHeight": treeHeight,
	})
	if err != nil {
		return xerrors.Errorf("failed to create routing struct: %v", err)
	}

	// For the moment this sender can only receive messages to itself
	// TODO: find a way to know the other nodes.
	sender := &sender{
		encoder: o.encoder,
		// This address is used when the client doesn't find the address it
		// should send the message to in the list of participant. In that case
		// it packs the message in an enveloppe and send it back to this
		// address, which is registered in the list of participant.
		// It is also used to indicate the "from" of the message in the case it
		// doesn't relay but sends directly.
		address:      address{rpcID},
		participants: map[string]overlayStream{rpcID: stream},
		name:         "remote RPC of " + o.addr.String(),
		srvCert:      o.srvCert,
		traffic:      o.traffic,
		routing:      routing,
	}

	receiver := receiver{
		encoder: o.encoder,
		in:      make(chan *OverlayMsg),
		errs:    make(chan error),
		name:    "remote RPC of " + o.addr.String(),
		traffic: o.traffic,
	}

	for _, addr := range routing.GetDirectLinks() {
		peer, found := o.neighbour[addr.String()]
		if !found {
			err = xerrors.Errorf("failed to find peer '%s' from the neighbours: %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			return err
		}

		clientConn, err := getConnection(addr.String(), peer, *o.srvCert)
		if err != nil {
			err = xerrors.Errorf("failed to get client conn for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			return err
		}
		cl := NewOverlayClient(clientConn)

		header := metadata.New(map[string]string{headerURIKey: apiURI[0]})
		newCtx := stream.Context()
		newCtx = metadata.NewOutgoingContext(newCtx, header)

		clientStream, err := cl.Stream(newCtx)
		if err != nil {
			err = xerrors.Errorf("failed to get stream for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			return err
		}

		sender.participants[addr.String()] = clientStream

		// Sending the routing info as first messages to our childs
		clientStream.Send(&OverlayMsg{Message: overlayMsg.Message})

		// Listen on the clients streams and notify the orchestrator or relay
		// messages
		go func(addr mino.Address) {
			for {
				err := listenStream(clientStream, &receiver, sender, addr)
				if err == io.EOF {
					return
				}
				status, ok := status.FromError(err)
				if ok && err != nil && status.Code() == codes.Canceled {
					return
				}
				if err != nil {
					err = xerrors.Errorf("failed to listen stream on child in overlay: %v", err)
					fabric.Logger.Err(err).Send()
					return
				}
			}
		}(addr)
	}

	// listen on my own stream
	go func() {

		for {
			err := listenStream(stream, &receiver, sender, o.addr)
			if err == io.EOF {
				<-ctx.Done()
				return
			}
			status, ok := status.FromError(err)
			if ok && err != nil && status.Code() == codes.Canceled {
				<-ctx.Done()
				return
			}
			if err != nil {
				err = xerrors.Errorf("failed to listen stream in overlay: %v", err)
				fabric.Logger.Err(err).Send()
				<-ctx.Done()
				return
			}
		}
	}()

	err = handler.Stream(sender, receiver)
	if err != nil {
		return xerrors.Errorf("failed to call the stream handler: %v", err)
	}

	<-ctx.Done()

	return nil

}
