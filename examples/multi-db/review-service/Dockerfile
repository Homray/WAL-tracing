# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/multi-db/review-service/ examples/multi-db/review-service/
WORKDIR /src/examples/multi-db/review-service
RUN go mod download
RUN go build -o /review-service .
FROM alpine:3.20
COPY --from=builder /review-service /review-service
ENTRYPOINT ["/review-service"]
