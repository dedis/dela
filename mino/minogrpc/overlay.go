package minogrpc

import (
	context "context"
	"crypto/tls"
	"io"
	"sync"

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
	mesh map[string]Peer
	// routing table from the server
	routingTable map[string]string
	// This certificate is used to create a new stream connection if possible
	srvCert *tls.Certificate
	// Used to record traffic activity
	traffic *traffic
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

	rpcID := "server_" + o.addr.String()

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
		name:         "remote RPC",
		mesh:         o.mesh,
		srvCert:      o.srvCert,
		traffic:      o.traffic,
	}

	receiver := receiver{
		encoder: o.encoder,
		in:      make(chan *OverlayMsg),
		errs:    make(chan error),
		name:    "remote RPC",
		traffic: o.traffic,
	}

	var peerWait sync.WaitGroup

	for _, peer := range o.mesh {
		addr := address{peer.Address}
		clientConn, err := getConnection(addr.String(), peer, *o.srvCert)
		if err != nil {
			err = xerrors.Errorf("failed to get client conn for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			return err
		}
		cl := NewOverlayClient(clientConn)

		header := metadata.New(map[string]string{headerURIKey: apiURI[0]})
		ctx = metadata.NewOutgoingContext(ctx, header)

		clientStream, err := cl.Stream(ctx)

		if err != nil {
			err = xerrors.Errorf("failed to get stream for client '%s': %v",
				addr.String(), err)
			fabric.Logger.Err(err).Send()
			return err
		}
		sender.participants[addr.String()] = clientStream

		// Listen on the clients streams and notify the orchestrator or relay
		// messages
		peerWait.Add(1)
		go func() {
			defer peerWait.Done()
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
					err = xerrors.Errorf("failed to listen stream: %v")
					fabric.Logger.Err(err).Send()
					return
				}
			}
		}()
	}

	// add the gateways based on the routing table
	for k, v := range o.routingTable {
		gateway, ok := sender.participants[v]
		if !ok {
			// TODO: handle this situation
		}
		sender.participants[k] = gateway
	}

	// listen on my own stream
	go func() {
		for {
			err := listenStream(stream, &receiver, sender, o.addr)
			if err == io.EOF {
				return
			}
			status, ok := status.FromError(err)
			if ok && err != nil && status.Code() == codes.Canceled {
				return
			}
			if err != nil {
				err = xerrors.Errorf("failed to listen stream: %v", err)
				fabric.Logger.Err(err).Send()
				return
			}
		}
	}()

	err := handler.Stream(sender, receiver)
	if err != nil {
		return xerrors.Errorf("failed to call the stream handler: %v", err)
	}

	peerWait.Wait()
	return nil

}
