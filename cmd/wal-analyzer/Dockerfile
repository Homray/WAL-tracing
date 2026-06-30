FROM golang:1.23-alpine AS builder

WORKDIR

COPY go.mod ./go.mod
COPY waltracer/ waltracer/

COPY cmd/wal-analyzer/ cmd/wal-analyzer/

WORKDIR /src/cmd/wal-analyzer
RUN go mod download
RUN go build -o /wal-analyzer .

FROM alpine:3.20
COPY --from=builder /wal-analyzer /wal-analyzer
ENTRYPOINT ["/wal-analyzer"]
