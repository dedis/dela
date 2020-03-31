generate:
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.3.4
	go generate ./...

lint:
	# Coding style static check.
	@go get -v honnef.co/go/tools/cmd/staticcheck
	@go mod tidy
	staticcheck ./...
