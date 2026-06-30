FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o /loadgen-orders .
FROM alpine:3.20
COPY --from=builder /loadgen-orders /loadgen-orders
ENTRYPOINT ["/loadgen-orders"]
