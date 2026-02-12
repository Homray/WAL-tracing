FROM golang:1.23-alpine

WORKDIR /repo
COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o repo

EXPOSE 8090
CMD ["./repo"]
