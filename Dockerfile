FROM golang:1.19 AS builder

RUN CGO_ENABLED=0 GOOS=linux go install github.com/stefanbode/vitotrol2mqtt@master
RUN CGO_ENABLED=0 GOOS=linux go install github.com/maxatome/go-vitotrol/cmd/vitotrol@master

FROM alpine:3.13.2

COPY --from=builder /go/bin/vitotrol2mqtt /vitotrol2mqtt
COPY --from=builder /go/bin/vitotrol /vitotrol
RUN mkdir /config
COPY ./vitotrol2mqtt.yml /config/vitotrol2mqtt.yml
COPY ./vitotrol2mqtt.yml /vitotrol2mqtt.yml

ENTRYPOINT ["/vitotrol2mqtt", "-config", "/config/vitotrol2mqtt.yml"]
