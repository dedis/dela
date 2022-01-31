// Package ptypes contains the protobuf definitions for the implementation of
// minogrpc.
package ptypes

//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative overlay.proto
