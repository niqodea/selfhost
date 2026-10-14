FROM docker.io/library/alpine:3.24.2\
@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache dnsmasq

ADD \
    --checksum=sha256:942acffdf433d2c5b856c8f6651aa174f04770e86a251609831dd285d3a0c60a \
    --chmod=644 \
    https://raw.githubusercontent.com/StevenBlack/hosts/af9c4bdc1d7919b4176b711c2636f9ff3fd75382/hosts \
    /etc/dnsmasq.d/blocklist.hosts

ENTRYPOINT ["dnsmasq"]
