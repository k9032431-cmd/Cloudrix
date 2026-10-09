VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/k9032431-cmd/cloudrix/internal/api.Version=$(VERSION)

.PHONY: all web build test dev clean

all: web build

web:
	cd web && npm ci && npm run build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/cloudrix ./cmd/cloudrix

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck

# Run the API on :8000 and the UI dev server on :5173 (proxying /api and /sub).
dev:
	CLOUDRIX_DB=dev.db CLOUDRIX_ADMIN_USERNAME=admin CLOUDRIX_ADMIN_PASSWORD=admin12345 go run ./cmd/cloudrix & \
	cd web && npm run dev

clean:
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg
