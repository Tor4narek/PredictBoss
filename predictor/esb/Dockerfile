FROM golang:1.24-alpine AS build

WORKDIR /src

COPY predictor/esb/go.mod predictor/esb/go.sum ./
COPY predictor/esb/cmd ./cmd
COPY predictor/esb/internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/esb \
    ./cmd/esb

FROM alpine:3.20

RUN apk add --no-cache wget \
    && addgroup -S esb \
    && adduser -S -G esb esb
COPY --from=build /out/esb /usr/local/bin/esb
COPY predictor/agents /app/agents
WORKDIR /app
ENV AGENTS_DIR=/app/agents

USER esb
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=3s CMD wget -q -O - http://127.0.0.1:8081/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/esb"]
