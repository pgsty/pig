FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY pig /usr/bin/pig
ENTRYPOINT ["/usr/bin/pig"]