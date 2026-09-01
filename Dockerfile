FROM golang:1.26.6-alpine3.24@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS builder
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
