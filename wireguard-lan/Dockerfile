# untagged, the only available image for now (version 2.80)
FROM docker.io/strm/dnsmasq\
@sha256:dcf4c0aeb69ea6b9bca81314449d732ecd2ea021588d8a34d4be7c2304f89a39

ADD \
    --checksum=sha256:942acffdf433d2c5b856c8f6651aa174f04770e86a251609831dd285d3a0c60a \
    --chmod=644 \
    https://raw.githubusercontent.com/StevenBlack/hosts/af9c4bdc1d7919b4176b711c2636f9ff3fd75382/hosts \
    /etc/dnsmasq/blocklist.hosts
