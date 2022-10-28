FROM golang:1.17-alpine as builder

WORKDIR /app

# COPY go.mod go.sum main.go /app/
COPY go.mod main.go /app/

ENV CGO_ENABLED=0
RUN go build -o /app/server /app/main.go

FROM alpine:3.12

COPY --from=builder /app/server /usr/local/bin/zipserver

CMD ["/usr/local/bin/zipserver"]
