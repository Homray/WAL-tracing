FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o /loadgen-executors .
FROM alpine:3.20
COPY --from=builder /loadgen-executors /loadgen-executors
ENTRYPOINT ["/loadgen-executors"]
