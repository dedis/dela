// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package ptypes

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// OverlayClient is the client API for Overlay service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type OverlayClient interface {
	// Join handles join request from an unknown node. It accepts to share the
	// certificates if the token is valid.
	Join(ctx context.Context, in *JoinRequest, opts ...grpc.CallOption) (*JoinResponse, error)
	// Share handles a certificate share from another participant of the
	// network.
	Share(ctx context.Context, in *Certificate, opts ...grpc.CallOption) (*CertificateAck, error)
	// Call is a unicast rpc to send a message to a participant and expect a
	// reply from it.
	Call(ctx context.Context, in *Message, opts ...grpc.CallOption) (*Message, error)
	// Stream is a stream rpc that will build a network of nodes which will
	// relay the messages between each others.
	Stream(ctx context.Context, opts ...grpc.CallOption) (Overlay_StreamClient, error)
	// Forward is used in association with Stream to send a message through
	// relays and get a feedback that the message has been received.
	Forward(ctx context.Context, in *Packet, opts ...grpc.CallOption) (*Ack, error)
}

type overlayClient struct {
	cc grpc.ClientConnInterface
}

func NewOverlayClient(cc grpc.ClientConnInterface) OverlayClient {
	return &overlayClient{cc}
}

func (c *overlayClient) Join(ctx context.Context, in *JoinRequest, opts ...grpc.CallOption) (*JoinResponse, error) {
	out := new(JoinResponse)
	err := c.cc.Invoke(ctx, "/ptypes.Overlay/Join", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *overlayClient) Share(ctx context.Context, in *Certificate, opts ...grpc.CallOption) (*CertificateAck, error) {
	out := new(CertificateAck)
	err := c.cc.Invoke(ctx, "/ptypes.Overlay/Share", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *overlayClient) Call(ctx context.Context, in *Message, opts ...grpc.CallOption) (*Message, error) {
	out := new(Message)
	err := c.cc.Invoke(ctx, "/ptypes.Overlay/Call", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *overlayClient) Stream(ctx context.Context, opts ...grpc.CallOption) (Overlay_StreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &Overlay_ServiceDesc.Streams[0], "/ptypes.Overlay/Stream", opts...)
	if err != nil {
		return nil, err
	}
	x := &overlayStreamClient{stream}
	return x, nil
}

type Overlay_StreamClient interface {
	Send(*Packet) error
	Recv() (*Packet, error)
	grpc.ClientStream
}

type overlayStreamClient struct {
	grpc.ClientStream
}

func (x *overlayStreamClient) Send(m *Packet) error {
	return x.ClientStream.SendMsg(m)
}

func (x *overlayStreamClient) Recv() (*Packet, error) {
	m := new(Packet)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *overlayClient) Forward(ctx context.Context, in *Packet, opts ...grpc.CallOption) (*Ack, error) {
	out := new(Ack)
	err := c.cc.Invoke(ctx, "/ptypes.Overlay/Forward", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// OverlayServer is the server API for Overlay service.
// All implementations must embed UnimplementedOverlayServer
// for forward compatibility
type OverlayServer interface {
	// Join handles join request from an unknown node. It accepts to share the
	// certificates if the token is valid.
	Join(context.Context, *JoinRequest) (*JoinResponse, error)
	// Share handles a certificate share from another participant of the
	// network.
	Share(context.Context, *Certificate) (*CertificateAck, error)
	// Call is a unicast rpc to send a message to a participant and expect a
	// reply from it.
	Call(context.Context, *Message) (*Message, error)
	// Stream is a stream rpc that will build a network of nodes which will
	// relay the messages between each others.
	Stream(Overlay_StreamServer) error
	// Forward is used in association with Stream to send a message through
	// relays and get a feedback that the message has been received.
	Forward(context.Context, *Packet) (*Ack, error)
	mustEmbedUnimplementedOverlayServer()
}

// UnimplementedOverlayServer must be embedded to have forward compatible implementations.
type UnimplementedOverlayServer struct {
}

func (UnimplementedOverlayServer) Join(context.Context, *JoinRequest) (*JoinResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Join not implemented")
}
func (UnimplementedOverlayServer) Share(context.Context, *Certificate) (*CertificateAck, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Share not implemented")
}
func (UnimplementedOverlayServer) Call(context.Context, *Message) (*Message, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Call not implemented")
}
func (UnimplementedOverlayServer) Stream(Overlay_StreamServer) error {
	return status.Errorf(codes.Unimplemented, "method Stream not implemented")
}
func (UnimplementedOverlayServer) Forward(context.Context, *Packet) (*Ack, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Forward not implemented")
}
func (UnimplementedOverlayServer) mustEmbedUnimplementedOverlayServer() {}

// UnsafeOverlayServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to OverlayServer will
// result in compilation errors.
type UnsafeOverlayServer interface {
	mustEmbedUnimplementedOverlayServer()
}

func RegisterOverlayServer(s grpc.ServiceRegistrar, srv OverlayServer) {
	s.RegisterService(&Overlay_ServiceDesc, srv)
}

func _Overlay_Join_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(JoinRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OverlayServer).Join(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/ptypes.Overlay/Join",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OverlayServer).Join(ctx, req.(*JoinRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Overlay_Share_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Certificate)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OverlayServer).Share(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/ptypes.Overlay/Share",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OverlayServer).Share(ctx, req.(*Certificate))
	}
	return interceptor(ctx, in, info, handler)
}

func _Overlay_Call_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Message)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OverlayServer).Call(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/ptypes.Overlay/Call",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OverlayServer).Call(ctx, req.(*Message))
	}
	return interceptor(ctx, in, info, handler)
}

func _Overlay_Stream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(OverlayServer).Stream(&overlayStreamServer{stream})
}

type Overlay_StreamServer interface {
	Send(*Packet) error
	Recv() (*Packet, error)
	grpc.ServerStream
}

type overlayStreamServer struct {
	grpc.ServerStream
}

func (x *overlayStreamServer) Send(m *Packet) error {
	return x.ServerStream.SendMsg(m)
}

func (x *overlayStreamServer) Recv() (*Packet, error) {
	m := new(Packet)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _Overlay_Forward_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Packet)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OverlayServer).Forward(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/ptypes.Overlay/Forward",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OverlayServer).Forward(ctx, req.(*Packet))
	}
	return interceptor(ctx, in, info, handler)
}

// Overlay_ServiceDesc is the grpc.ServiceDesc for Overlay service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Overlay_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ptypes.Overlay",
	HandlerType: (*OverlayServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Join",
			Handler:    _Overlay_Join_Handler,
		},
		{
			MethodName: "Share",
			Handler:    _Overlay_Share_Handler,
		},
		{
			MethodName: "Call",
			Handler:    _Overlay_Call_Handler,
		},
		{
			MethodName: "Forward",
			Handler:    _Overlay_Forward_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Stream",
			Handler:       _Overlay_Stream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "overlay.proto",
}