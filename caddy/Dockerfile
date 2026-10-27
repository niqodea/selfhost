FROM docker.io/library/caddy:2.11-builder\
@sha256:d69b881a45e9778ef5a5a664ec5325fc707db8bc36f6a3a321590133164ea24b \
    AS builder

ARG CADDY_DNS_PLUGIN
RUN xcaddy build \
    --with "$CADDY_DNS_PLUGIN"

FROM caddy:2.11

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
