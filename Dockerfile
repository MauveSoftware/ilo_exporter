FROM golang:1.27rc2-alpine3.24@sha256:dcbb18cc5fa1082364dc6aa95224b6b55429d09cbb9631a053d8064c1c367300 AS builder
ADD . /go/ilo_exporter/
WORKDIR /go/ilo_exporter
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/ilo_exporter

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
ENV API_USERNAME=''
ENV API_PASSWORD=''
ENV API_MAX_CONCURRENT='4'
ENV CMD_FLAGS=''
RUN apk --no-cache add ca-certificates bash
COPY --from=builder /go/bin/ilo_exporter /app/ilo_exporter
EXPOSE 19545
ENTRYPOINT /app/ilo_exporter -api.username=$API_USERNAME -api.password=$API_PASSWORD -api.max-concurrent-requests=$API_MAX_CONCURRENT $CMD_FLAGS
