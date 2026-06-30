# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/chain/api-gateway/ examples/chain/api-gateway/
WORKDIR /src/examples/chain/api-gateway
RUN go mod download
RUN go build -o /api-gateway .
FROM alpine:3.20
COPY --from=builder /api-gateway /api-gateway
ENTRYPOINT ["/api-gateway"]
