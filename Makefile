.PHONY: all tidy generate lint vet test coverage pushdoc

# Default "make" target to check locally that everything is ok, BEFORE pushing remotely
all: lint vet test
	@echo "Done with the standard checks"

tidy:
	go mod tidy

generate: tidy
	go get -u google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
	go get -u google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go get -u google.golang.org/genproto/googleapis/rpc
	go generate ./...

lint: tidy
	@go get github.com/golangci/golangci-lint/cmd/golangci-lint
	golangci-lint run

tests:
	while make test; do echo "Testing again at $$(date)"; done; echo "Failed testing"

FLAKY_TESTS_PBFT := (TestService_Scenario_Basic|TestService_Scenario_ViewChange|TestService_Scenario_FinalizeFailure)
FLAKY_TESTS_MINOWS := (Test_session_Recv_SessionEnded)

# test runs all tests in DELA without coverage
# It first runs all the tests in "short" mode, so the flaky tests don't run.
# Then the flaky tests get run separately for at most 3 times, and hopefully it all works out.
test: tidy
	go test ./... -short -count=1 || exit 1
	@for count in $$( seq 4 ); do \
		if [ "$$count" -eq 4 ]; then \
			echo "Couldn't run all flaky tests in 3 tries"; \
			exit 1; \
		fi; \
		echo "Running $$count/3"; \
		if ! go test -count=1 ./mino/minows -run="${FLAKY_TESTS_MINOWS}"; then \
			continue; \
		fi; \
		if ! go test -count=1 ./core/ordering/cosipbft -run="${FLAKY_TESTS_PBFT}"; then \
			continue; \
		fi; \
		break; \
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
