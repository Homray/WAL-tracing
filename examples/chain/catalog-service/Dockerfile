# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/chain/catalog-service/ examples/chain/catalog-service/
WORKDIR /src/examples/chain/catalog-service
RUN go mod download
RUN go build -o /catalog-service .
FROM alpine:3.20
COPY --from=builder /catalog-service /catalog-service
ENTRYPOINT ["/catalog-service"]
