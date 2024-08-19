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
	@go install honnef.co/go/tools/cmd/staticcheck@v0.4.7
	staticcheck `go list ./... | grep -Ev "(go\.dedis\.ch/dela/internal/testing|go\.dedis\.ch/dela/mino/minogrpc/ptypes)"`

vet: tidy
	@echo "⚠️ Warning: the following only works with go >= 1.14" && \
	go install ./internal/mcheck && \
	go vet -vettool=`go env GOPATH`/bin/mcheck -commentLen -ifInit ./...

tests:
	while make test; do echo "Testing again at $$(date)"; done; echo "Failed testing"

FLAKY_TESTS := (TestService_Scenario_Basic|TestService_Scenario_ViewChange|TestService_Scenario_FinalizeFailure)

# test runs all tests in DELA without coverage
# It first runs all the tests in "short" mode, so the flaky tests don't run.
# Then the flaky tests get run separately for at most 3 times, and hopefully it all works out.
test: tidy
	go test ./... -short -count=1 || exit 1
	@for count in $$( seq 3 ); do \
		echo "Running $$count/3"; \
		if go test -count=1 ./core/ordering/cosipbft -run="${FLAKY_TESTS}"; then \
			break; \
		fi; \
		if [[ $$count == 3 ]]; then \
			echo "Couldn't run all flaky tests in 3 tries"; \
			exit 1; \
		fi; \
	done

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
