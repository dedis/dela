gover=`go version | sed 's/.*go[0-9]\{1,\}\.\([0-9]\{2,\}\)\..*/\1/'`

generate:
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.3.4
	go generate ./...

lint:
	# Coding style static check.
	@go get -v honnef.co/go/tools/cmd/staticcheck
	@go mod tidy
	staticcheck ./...

vet:
	@# `go vet` with our custom check encouters a known bug with go < 14
	@if [ ${gover} -ge 14 ] ; then \
		go install ./internal/mcheck && \
		go vet -vettool=`go env GOPATH`/bin/mcheck -commentLen -ifInit ./...; \
	else \
		echo "please use go >= 14" && exit 1; \
	fi