FROM golang:1.23-alpine

WORKDIR /svc
COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o order-service

EXPOSE 8081
CMD ["./order-service"]
