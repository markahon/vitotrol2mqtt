FROM golang:1.19 AS builder

RUN CGO_ENABLED=0 GOOS=linux go install github.com/markahon/vitotrol2mqtt@master

FROM alpine:3.13.2

COPY --from=builder /go/bin/vitotrol2mqtt /vitotrol2mqtt

ENTRYPOINT ["/vitotrol2mqtt", "-config", "/vitotrol2mqtt.yml"]
