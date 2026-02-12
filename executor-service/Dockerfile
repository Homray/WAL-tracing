FROM golang:1.23-alpine

WORKDIR /svc
COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o executor-service

EXPOSE 8082
CMD ["./executor-service"]
