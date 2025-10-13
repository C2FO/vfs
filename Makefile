GOBIN ?= $$(go env GOPATH)/bin
MODULES := $(shell find . -type f -name "go.mod" -exec dirname {} \;)

.PHONY: lint
lint: $(addprefix lint/,$(MODULES))
lint/%:
	@echo "Running golangci-lint in $*/"
	@cd $* && golangci-lint run --build-tags=vfsintegration

.PHONY: test
test: $(addprefix test/,$(MODULES))
test/%:
	@echo "Running tests in $*/"
	@cd $* && go test ./...

.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@latest

.PHONY: check-coverage
check-coverage: install-go-test-coverage
	go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...
	${GOBIN}/go-test-coverage --config=./.testcoverage.yml
