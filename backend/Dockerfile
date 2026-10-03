FROM golang:1.27.0-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION
ARG COMMIT
ARG DATE
RUN test "$VERSION" = "$(sh scripts/resolve-version.sh)" \
    && printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-r[1-9][0-9]*$' \
    && printf '%s' "$COMMIT" | grep -Eq '^[[:xdigit:]]{40}$' \
    && printf '%s' "$DATE" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags="-s -w -X main.Version=$VERSION -X main.Commit=$COMMIT -X main.Date=$DATE -X main.BuildType=release" \
    -o /out/main ./cmd/server

FROM postgres:18-alpine AS postgres-client

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata libpq zstd-libs lz4-libs krb5-libs libldap libedit \
    && addgroup -g 1000 app && adduser -D -u 1000 -G app app
WORKDIR /app
ARG VERSION
ARG COMMIT
ARG DATE
LABEL org.opencontainers.image.source="https://github.com/ravicc02/sshzyu" \
    org.opencontainers.image.version="$VERSION" \
    org.opencontainers.image.revision="$COMMIT" \
    org.opencontainers.image.created="$DATE" \
    com.sshzy.distribution="sshzy" \
    com.sshzy.build-type="release"
COPY --from=postgres-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=postgres-client /usr/local/bin/pg_restore /usr/local/bin/pg_restore
COPY --from=postgres-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=postgres-client /usr/local/lib/libpq.so.5* /usr/local/lib/
COPY --from=builder --chown=app:app /out/main /app/main
COPY --from=builder --chown=app:app /src/resources /app/resources
RUN mkdir -p /app/data && chown app:app /app/data
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -q -T 4 -O /dev/null http://127.0.0.1:${SERVER_PORT:-8080}/health || exit 1
CMD ["/app/main"]
