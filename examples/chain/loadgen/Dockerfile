FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o /loadgen .
FROM alpine:3.20
COPY --from=builder /loadgen /loadgen
ENTRYPOINT ["/loadgen"]
