FROM golang:1.24.1-alpine3.21 AS builder

WORKDIR /app
COPY . .
RUN go build -o secfeed cmd/secfeed/main.go

FROM alpine:3.21

COPY --from=builder /app/secfeed /app/secfeed

WORKDIR /app

ENTRYPOINT ["./secfeed"]