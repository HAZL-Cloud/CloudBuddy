# CloudBuddy build and development tasks.

GO ?= go
BIN := bin/cloudbuddy

.PHONY: all build web release dev db-up db-down test test-go test-web vet adapters-aws clean

all: build

## Build the server (serves web/dist from disk; run `make web` first).
build:
	$(GO) build -o $(BIN) ./cmd/cloudbuddy

## Build the frontend into web/dist.
web:
	cd web && npm ci && npm run build

## Single binary with the frontend embedded.
release: web
	$(GO) build -tags embedweb -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/cloudbuddy

## Start the database (PostgreSQL in Docker on 127.0.0.1:54329).
db-up:
	docker compose up -d --wait

db-down:
	docker compose down

## Run the server in dev mode (accepts the Vite dev server origin).
dev: build
	$(BIN) serve --dev

test: test-go test-web

## Go tests. Database tests need `make db-up`; they skip without it.
test-go:
	$(GO) test ./...

test-web:
	cd web && npm test

vet:
	$(GO) vet ./...
	gofmt -l cmd internal adapters web/*.go tools

## Regenerate adapters/aws-generated.yaml from the installed AWS CLI v2.
adapters-aws:
	@MODELS=$$(python3 -c 'import awscli, os; print(os.path.join(os.path.dirname(awscli.__file__), "botocore", "data"))' 2>/dev/null || find /opt/homebrew/Cellar/awscli /usr/local/aws-cli -type d -path '*botocore/data' 2>/dev/null | head -1); \
	VERSION=$$(aws --version 2>&1 | cut -d' ' -f1 | cut -d/ -f2); \
	$(GO) run ./tools/adaptergen -models "$$MODELS" -version "$$VERSION" -out adapters/aws-generated.yaml

clean:
	rm -rf bin web/dist
