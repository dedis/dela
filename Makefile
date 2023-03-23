.PHONY: all tidy generate lint vet test coverage pushdoc

# Default "make" target to check locally that everything is ok, BEFORE pushing remotely
all: lint vet test
	@echo "Done with the standard checks"

tidy:
	go mod tidy

generate: tidy
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.3.5
	go generate ./...

# Some packages are excluded from staticcheck due to deprecated warnings: #208.
lint: tidy
	# Coding style static check.
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck `go list ./... | grep -Ev "(go\.dedis\.ch/dela/internal/testing|go\.dedis\.ch/dela/mino/minogrpc/ptypes)"`

vet: tidy
	@echo "⚠️ Warning: the following only works with go >= 1.14" && \
	go install ./internal/mcheck && \
	go vet -vettool=`go env GOPATH`/bin/mcheck -commentLen -ifInit ./...

# test runs all tests in DELA without coverage
test: tidy
	go test ./...

# test runs all tests in DELA and generate a coverage output (to be used by sonarcloud)
coverage: tidy
	go test -json -covermode=count -coverprofile=profile.cov ./... | tee report.json

# https://pkg.go.dev/go.dedis.ch/dela needs to be updated on the Go proxy side
# to get the latest master. This command refreshes the proxy with the latest
# commit on the upstream master branch.
# Note: CURL must be installed
pushdoc:
	@echo "Requesting the proxy..."
	@curl "https://proxy.golang.org/go.dedis.ch/dela/@v/$(shell git log origin/master -1 --format=format:%H).info"
	@echo "\nDone."