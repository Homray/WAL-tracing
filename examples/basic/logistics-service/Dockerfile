# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/basic/logistics-service/ examples/basic/logistics-service/
WORKDIR /src/examples/basic/logistics-service
RUN go mod download
RUN go build -o /logistics-service .
FROM alpine:3.20
COPY --from=builder /logistics-service /logistics-service
ENTRYPOINT ["/logistics-service"]
