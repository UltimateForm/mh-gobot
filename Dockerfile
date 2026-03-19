FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o .out/mh-gobot .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/.out/mh-gobot .
RUN mkdir -p /root/.mh-gobot
CMD ["./mh-gobot"]
