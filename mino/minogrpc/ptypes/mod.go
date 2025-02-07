// Package ptypes contains the protobuf definitions for the implementation of
// minogrpc.
// To re-generate, install protoc and then run
// 
// go generate .
package ptypes

//go:generate protoc -I ./ --go_out=./ --go-grpc_out=./ ./overlay.proto
