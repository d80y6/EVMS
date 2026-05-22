FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
# COPY go.sum ./
RUN go mod download
COPY . .
RUN go build -o main ./services/ingest/main.go

FROM alpine:latest
RUN apk add --no-cache ffmpeg
WORKDIR /root/
COPY --from=builder /app/main .
CMD ["./main"]
