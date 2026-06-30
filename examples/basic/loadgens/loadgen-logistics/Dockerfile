FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o /loadgen-logistics .
FROM alpine:3.20
COPY --from=builder /loadgen-logistics /loadgen-logistics
ENTRYPOINT ["/loadgen-logistics"]
