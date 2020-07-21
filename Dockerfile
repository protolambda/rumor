FROM golang:alpine as build

RUN apk add --no-cache ca-certificates build-base

WORKDIR /build

ADD . .

RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags '-extldflags "-static"' -o app

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt \
     /etc/ssl/certs/ca-certificates.crt

COPY --from=build /build/app /app

ENTRYPOINT ["/app"]
