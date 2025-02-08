// Package ptypes contains the protobuf definitions for the implementation of
// minogrpc.
package ptypes

//go:generate protoc -I ./ --go_out=./ --go-grpc_out=./ ./overlay.proto
