FROM alpine:3.19.3

ARG ROOT
ENV ROOT=${ROOT}

RUN apk add --no-cache git git-daemon lighttpd

COPY .build/lighttpd.conf /etc/lighttpd/lighttpd.conf

EXPOSE 80

CMD ["lighttpd", "-D", "-f", "/etc/lighttpd/lighttpd.conf"]
