# Build context: project root
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/
COPY examples/multi-db/user-service/ examples/multi-db/user-service/
WORKDIR /src/examples/multi-db/user-service
RUN go mod download
RUN go build -o /user-service .
FROM alpine:3.20
COPY --from=builder /user-service /user-service
ENTRYPOINT ["/user-service"]
