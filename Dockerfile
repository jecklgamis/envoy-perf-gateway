FROM envoyproxy/envoy:v1.39-latest
RUN apt update -y && apt install -y curl dumb-init supervisor python3 python3-pip && \
    pip3 install --no-cache-dir requests boto3 flask && \
    rm -rf /var/lib/apt/lists/*

COPY supervisor.ini /etc/supervisor.d/
RUN mkdir -p /var/log/supervisor

COPY app.py /
COPY run-app.sh /

COPY run-envoy.sh /
COPY config/envoy.yaml /etc/envoy/envoy.yaml
RUN mkdir -p /etc/envoy/dynamic
# Seed the dynamic dir so the image is self-contained; config_fetcher.py
# (below) takes over from here at runtime, polling HTTP or S3 and writing
# into this same directory from inside the container.
COPY rendered/ /etc/envoy/dynamic/

COPY fetcher/ /fetcher/
COPY server.crt /etc/
COPY server.key /etc/

EXPOSE 8080
EXPOSE 8443
EXPOSE 9901

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/usr/bin/supervisord","-c","/etc/supervisor.d/supervisor.ini"]
