FROM alpine:3.20

RUN addgroup -S hermes && adduser -S -G hermes hermes
WORKDIR /app
COPY config.yaml /etc/hermes/config.yaml

USER hermes

# Временный runtime до подключения настоящего Hermes runner.
CMD ["/bin/sh", "-c", "echo 'Hermes agent config loaded from /etc/hermes/config.yaml'; exec sleep infinity"]
