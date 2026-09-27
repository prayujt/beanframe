PYTHON := $(CURDIR)/.venv/bin/python

.PHONY: setup setup-deps check test build
setup: setup-deps
	pnpm --filter @beanframe/web exec playwright install chromium

setup-deps:
	python3 -m venv .venv
	$(PYTHON) -m pip install -e packages/beancount-engine
	pnpm install --frozen-lockfile

check:
	python3 -m unittest discover -s scripts -p 'test_*.py' -v
	$(PYTHON) -m unittest discover -s packages/beancount-engine/tests -v
	cd apps/server && TEST_PYTHON=$(PYTHON) go test -race -count=1 ./...
	cd apps/server && go vet ./...
	pnpm check:web

test: check
	$(MAKE) build
	pnpm test:web

build:
	pnpm build:web
	cd apps/server && go build -o ../../bin/beanframe ./cmd/server

.PHONY: proto-tools proto check-proto
proto-tools:
	GOBIN=$(CURDIR)/.tools go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
	GOBIN=$(CURDIR)/.tools go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.21.0

proto: proto-tools
	pnpm exec buf lint
	PATH="$(CURDIR)/.tools:$$PATH" pnpm exec buf generate

check-proto: proto
	git diff --exit-code -- apps/server/gen apps/web/src/lib/gen
