FROM golang:1.24-alpine

WORKDIR /wal-tracer
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o wal-tracer

CMD ["./wal-tracer"]
