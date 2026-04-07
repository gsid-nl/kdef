VERSION ?= 0.2.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "nogit")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
           -X github.com/gsid-nl/kdef/internal/version.Version=$(VERSION) \
           -X github.com/gsid-nl/kdef/internal/version.Commit=$(COMMIT) \
           -X github.com/gsid-nl/kdef/internal/version.Date=$(DATE)

.PHONY: build test clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o kdef ./cmd/kdef

test:
	go test ./... -v

clean:
	rm -f kdef
