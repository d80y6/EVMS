FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o main ./services/ingest/main.go

FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
CMD ["./main"]
