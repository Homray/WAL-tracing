# Build context must be the project root (set in docker-compose.yml).
FROM golang:1.23-alpine AS builder

WORKDIR /src

# Copy the waltracer library (cached layer – changes rarely)
COPY go.mod go.sum* ./
COPY waltracer/ waltracer/

# Copy this service
COPY examples/basic/repo/ examples/basic/repo/

WORKDIR /src/examples/basic/repo
RUN go mod download
RUN go build -o /repo .

FROM alpine:3.20
COPY --from=builder /repo /repo
ENTRYPOINT ["/repo"]
