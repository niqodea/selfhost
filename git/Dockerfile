FROM alpine:3.19.3

ARG ROOT
ENV ROOT=${ROOT}

RUN apk add --no-cache git git-daemon lighttpd

# The server has no login of its own, and git-http-backend refuses a push from
# a client that has not logged in unless this is set
RUN git config --system http.receivepack true

COPY .build/lighttpd.conf /etc/lighttpd/lighttpd.conf

EXPOSE 80

CMD ["lighttpd", "-D", "-f", "/etc/lighttpd/lighttpd.conf"]
