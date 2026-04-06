FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o .out/mh-gobot .

FROM alpine:latest
RUN adduser -D -h /home/appuser appuser
WORKDIR /app
COPY --from=builder /app/.out/mh-gobot .
RUN mkdir -p /home/appuser/.mh-gobot
RUN chown -R appuser:appuser /home/appuser/.mh-gobot
USER appuser
CMD ["./mh-gobot"]
