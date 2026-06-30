# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/chain/store-service/ examples/chain/store-service/
WORKDIR /src/examples/chain/store-service
RUN go mod download
RUN go build -o /store-service .
FROM alpine:3.20
COPY --from=builder /store-service /store-service
ENTRYPOINT ["/store-service"]
