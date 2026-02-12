FROM golang:1.23-alpine

WORKDIR /loadgen
COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o loadgen-logistics

CMD ["./loadgen-logistics"]
