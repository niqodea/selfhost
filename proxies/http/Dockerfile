FROM docker.io/library/alpine:3.24.2\
@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache tinyproxy

# A list entry must match a whole host name, so a name matches only itself,
# and tunnels to a listed host go only to its HTTPS port
COPY <<EOF /etc/tinyproxy/tinyproxy.conf
Port 3128
Filter "/etc/tinyproxy/domains.txt"
FilterType fnmatch
FilterDefaultDeny Yes
ConnectPort 443
EOF

USER nobody
ENTRYPOINT ["tinyproxy", "-d"]
