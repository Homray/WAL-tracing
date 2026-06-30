# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/basic/order-service/ examples/basic/order-service/
WORKDIR /src/examples/basic/order-service
RUN go mod download
RUN go build -o /order-service .
FROM alpine:3.20
COPY --from=builder /order-service /order-service
ENTRYPOINT ["/order-service"]
