FROM golang:1.24-bookworm AS builder
WORKDIR /build

COPY app/go.mod app/main.go ./app/
RUN cd app && go build -o /out/app .

COPY fetcher/go.mod fetcher/go.sum fetcher/main.go ./fetcher/
RUN cd fetcher && go build -o /out/fetcher .

FROM envoyproxy/envoy:v1.39-latest
RUN apt update -y && apt install -y curl dumb-init supervisor && rm -rf /var/lib/apt/lists/*

COPY supervisor.ini /etc/supervisor.d/
RUN mkdir -p /var/log/supervisor

COPY --from=builder /out/app /app
COPY run-app.sh /

COPY run-envoy.sh /
COPY config/envoy.yaml /etc/envoy/envoy.yaml
RUN mkdir -p /etc/envoy/dynamic
# Seed the dynamic dir so the image is self-contained; config-fetcher
# (below) takes over from here at runtime, polling HTTP or S3 and writing
# into this same directory from inside the container.
COPY rendered/ /etc/envoy/dynamic/
# layered_runtime's disk_layer (config/envoy.yaml) reads one file per
# runtime key from here; empty until config-fetcher expands the first
# rendered/runtime.yaml it fetches, which is the correct baseline (no
# fault overrides) for a fresh image.
RUN mkdir -p /etc/envoy/dynamic/runtime/current

RUN mkdir -p /fetcher
COPY --from=builder /out/fetcher /fetcher/fetcher
COPY fetcher/run-fetcher.sh /fetcher/

EXPOSE 8080
EXPOSE 9901

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/usr/bin/supervisord","-c","/etc/supervisor.d/supervisor.ini"]
