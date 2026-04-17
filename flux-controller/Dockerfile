FROM golang:1.25 AS builder

WORKDIR /workspace

# Copy go module files first for cache
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY cmd/ cmd/
COPY internal/ internal/
COPY flux-controller/ flux-controller/

# Build the controller
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o kdef-controller ./flux-controller/cmd/

# Runtime image
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /workspace/kdef-controller /kdef-controller

USER 65532:65532
ENTRYPOINT ["/kdef-controller"]
