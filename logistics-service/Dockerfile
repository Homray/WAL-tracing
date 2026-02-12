FROM golang:1.23-alpine

WORKDIR /svc
COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o logistics-service

EXPOSE 8083
CMD ["./logistics-service"]
