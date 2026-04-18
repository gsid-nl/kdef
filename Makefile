VERSION ?= 0.6.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "nogit")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
           -X github.com/gsid-nl/kdef/internal/version.Version=$(VERSION) \
           -X github.com/gsid-nl/kdef/internal/version.Commit=$(COMMIT) \
           -X github.com/gsid-nl/kdef/internal/version.Date=$(DATE)

DIST    := dist
NFPM    := $(shell go env GOPATH)/bin/nfpm

.PHONY: build test clean build-all completions package argocd-plugin

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o kdef ./cmd/kdef
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o kdef-lsp ./cmd/kdef-lsp

test:
	go test ./... -v

clean:
	rm -f kdef kdef-lsp
	rm -rf $(DIST)

build-all: clean
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-linux-amd64           ./cmd/kdef
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-linux-arm64           ./cmd/kdef
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-darwin-amd64          ./cmd/kdef
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-darwin-arm64          ./cmd/kdef
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-windows-amd64.exe     ./cmd/kdef
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-lsp-linux-amd64       ./cmd/kdef-lsp
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-lsp-linux-arm64       ./cmd/kdef-lsp
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-lsp-darwin-amd64      ./cmd/kdef-lsp
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-lsp-darwin-arm64      ./cmd/kdef-lsp
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST)/kdef-lsp-windows-amd64.exe ./cmd/kdef-lsp
	@echo "Built $(VERSION) binaries in $(DIST)/"
	@ls -lh $(DIST)/

completions: build
	@mkdir -p completions
	./kdef completion bash > completions/kdef.bash
	./kdef completion zsh  > completions/kdef.zsh
	./kdef completion fish > completions/kdef.fish

argocd-plugin: build-all
	cp $(DIST)/kdef-linux-amd64 argocd-plugin/kdef

package: build-all completions
	@# Generate resolved nfpm configs per arch, then build packages
	@for arch in amd64 arm64; do \
		export VERSION=$(VERSION) GOARCH=$$arch; \
		envsubst < nfpm.yaml > $(DIST)/nfpm-$$arch.yaml; \
		for fmt in deb rpm apk; do \
			$(NFPM) package --config $(DIST)/nfpm-$$arch.yaml --packager $$fmt --target $(DIST)/; \
		done; \
	done
	@rm -f $(DIST)/nfpm-*.yaml
	@echo "Packages built in $(DIST)/"
	@ls -lh $(DIST)/*.deb $(DIST)/*.rpm $(DIST)/*.apk
