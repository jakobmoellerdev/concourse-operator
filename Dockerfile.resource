# Build the resource binary
FROM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the source code
COPY cmd/resource/ cmd/resource/

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /bin/resource ./cmd/resource/

# Final minimal container image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /bin/resource /opt/resource/check
COPY --from=builder /bin/resource /opt/resource/in
COPY --from=builder /bin/resource /opt/resource/out

USER 65532:65532

ENTRYPOINT ["/opt/resource/check"]
