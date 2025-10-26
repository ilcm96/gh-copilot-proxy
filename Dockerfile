FROM golang:1.25.3 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /src/gh-copilot-proxy ./cmd/server

FROM redhat/ubi10-minimal:latest

WORKDIR /app

RUN microdnf update -y && \
    microdnf install -y ca-certificates tzdata && \
    microdnf clean all

COPY --from=builder /src/gh-copilot-proxy /usr/local/bin/gh-copilot-proxy

EXPOSE 4000

ENTRYPOINT ["/usr/local/bin/gh-copilot-proxy"]
