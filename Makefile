generate:
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.3.5
	go generate ./...

# Some packages are exluded from staticcheck due to deprecated warnings: #208.
lint:
	# Coding style static check.
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go mod tidy
	staticcheck `go list ./... | grep -Ev "(go\.dedis\.ch/dela/internal/testing|go\.dedis\.ch/dela/mino/minogrpc/ptypes)"`

vet:
	@echo "⚠️ Warning: the following only works with go >= 1.14" && \
	go install ./internal/mcheck && \
	go vet -vettool=`go env GOPATH`/bin/mcheck -commentLen -ifInit ./...

# target to run all the possible checks; it's a good habit to run it before
# pushing code
check: lint vet
	go test ./...

# https://pkg.go.dev/go.dedis.ch/dela needs to be updated on the Go proxy side
# to get the latest master. This command refreshes the proxy with the latest
# commit on the upstream master branch.
# Note: CURL must be installed
pushdoc:
	@echo "Requesting the proxy..."
	@curl "https://proxy.golang.org/go.dedis.ch/dela/@v/$(shell git log origin/master -1 --format=format:%H).info"
	@echo "\nDone."