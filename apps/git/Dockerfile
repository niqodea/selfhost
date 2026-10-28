FROM docker.io/library/alpine:3.24.2\
@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

ARG ROOT
ENV ROOT=${ROOT}

RUN apk add --no-cache git git-daemon lighttpd

# The server has no login of its own, and git-http-backend refuses a push from
# a client that has not logged in unless this is set
RUN git config --system http.receivepack true

COPY .build/lighttpd.conf /etc/lighttpd/lighttpd.conf

EXPOSE 80

CMD ["lighttpd", "-D", "-f", "/etc/lighttpd/lighttpd.conf"]
