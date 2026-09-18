FROM golang:1.24-bookworm AS builder
WORKDIR /build

COPY go.mod main.go ./
RUN go build -o /out/config_server .

FROM debian:bookworm-slim
WORKDIR /app

COPY --from=builder /out/config_server .

ENV CONFIG_SERVER_STORAGE_DIR=/data
RUN mkdir -p /data

EXPOSE 8090
CMD ["./config_server"]
