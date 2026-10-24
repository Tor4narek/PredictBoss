FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd

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

USER esb
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=3s CMD wget -q -O - http://127.0.0.1:8081/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/esb"]
