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

GOMARKDOC := go run github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0
# package-dir:output-file pairs for the generated API docs in docs/
DOC_PAGES := \
	.:docs/vfs.md \
	backend:docs/backend.md \
	backend/azure:docs/azure.md \
	backend/ftp:docs/ftp.md \
	backend/gs:docs/gs.md \
	backend/mem:docs/mem.md \
	backend/os:docs/os.md \
	backend/s3:docs/s3.md \
	backend/sftp:docs/sftp.md \
	list:docs/list.md \
	utils:docs/utils.md \
	utils/authority:docs/authority.md \
	vfssimple:docs/vfssimple.md \
	vfscp:docs/vfscp.md \
	options:docs/options.md \
	options/delete:docs/delete_options.md \
	options/newfile:docs/newfile_options.md

.PHONY: docs
docs:
	@for page in $(DOC_PAGES); do \
		$(GOMARKDOC) --output "$${page#*:}" "./$${page%%:*}" || exit 1; \
	done

.PHONY: docs-check
docs-check:
	@for page in $(DOC_PAGES); do \
		$(GOMARKDOC) --check --output "$${page#*:}" "./$${page%%:*}" || { echo "$${page#*:} is out of date; run 'make docs'"; exit 1; }; \
	done

.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@latest

.PHONY: check-coverage
check-coverage: install-go-test-coverage
	go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...
	${GOBIN}/go-test-coverage --config=./.testcoverage.yml
