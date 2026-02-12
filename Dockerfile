FROM golang:1.25 AS builder

WORKDIR /app/

COPY go.* /app/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go mod download

COPY . /app/

RUN go test -v .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o alibaba-exporter .

FROM alpine:3.23.3

WORKDIR /app
COPY --from=builder /app/alibaba-exporter .

EXPOSE 8080
ENTRYPOINT ["/app/alibaba-exporter"]




