FROM golang:1.23-alpine

WORKDIR /loadgen
COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o loadgen-orders

CMD ["./loadgen-orders"]
