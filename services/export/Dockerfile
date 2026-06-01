FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /export services/export/main.go

FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /export /usr/local/bin/export
VOLUME ["/recordings", "/exports"]
EXPOSE 8094
CMD ["export"]
