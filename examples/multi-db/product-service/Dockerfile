# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/multi-db/product-service/ examples/multi-db/product-service/
WORKDIR /src/examples/multi-db/product-service
RUN go mod download
RUN go build -o /product-service .
FROM alpine:3.20
COPY --from=builder /product-service /product-service
ENTRYPOINT ["/product-service"]
