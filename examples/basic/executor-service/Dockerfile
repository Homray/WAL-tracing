# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/basic/executor-service/ examples/basic/executor-service/
WORKDIR /src/examples/basic/executor-service
RUN go mod download
RUN go build -o /executor-service .
FROM alpine:3.20
COPY --from=builder /executor-service /executor-service
ENTRYPOINT ["/executor-service"]
