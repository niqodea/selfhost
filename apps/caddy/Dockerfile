FROM caddy:2.11-builder AS builder

RUN xcaddy build \
    --with github.com/caddy-dns/dynu@v1.0.0

FROM caddy:2.11

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
