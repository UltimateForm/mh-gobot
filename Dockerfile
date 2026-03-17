FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o .out/ryard .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/.out/ryard .
COPY .env* ./
RUN mkdir -p /root/.ryard
CMD ["./ryard"]
