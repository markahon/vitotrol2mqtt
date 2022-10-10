FROM golang:1.18 AS builder

RUN CGO_ENABLED=0 GOOS=linux go get -u -a -tags netgo -ldflags '-w -extldflags "-static"' github.com/markahon/vitotrol2mqtt

FROM alpine:3.13.2

COPY --from=builder /go/bin/vitotrol2mqtt /vitotrol2mqtt

ENTRYPOINT ["/vitotrol2mqtt", "-config", "/vitotrol2mqtt.yml"]
