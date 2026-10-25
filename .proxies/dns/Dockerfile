FROM docker.io/library/alpine:3.24.2\
@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache dnsmasq

# dnsmasq has no upstream server of its own, so it answers only the names that
# an app's compose file gives it with --server, and refuses every other name
ENTRYPOINT ["dnsmasq", "--keep-in-foreground", "--no-resolv"]
