FROM docker.io/library/golang:1.27.3-alpine3.24\
@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build

WORKDIR /src
COPY .build/go.mod ./
COPY .build/*.go ./
# Without cgo, the binary needs no C library
RUN CGO_ENABLED=0 go build -o /importling .

FROM docker.io/library/alpine:3.24.2\
@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

COPY --from=build /importling /usr/local/bin/importling

EXPOSE 8080

ENTRYPOINT ["importling"]
