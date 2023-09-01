FROM golang as builder
ADD . /go/ilo_exporter/
WORKDIR /go/ilo_exporter
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/ilo_exporter

FROM alpine:latest
ENV API_USERNAME ''
ENV API_PASSWORD ''
ENV API_MAX_CONCURRENT '4'
ENV CMD_FLAGS ''
RUN apk --no-cache add ca-certificates bash
COPY --from=builder /go/bin/ilo_exporter /app/ilo_exporter
EXPOSE 19545
ENTRYPOINT /app/ilo_exporter -api.username=$API_USERNAME -api.password=$API_PASSWORD -api.max-concurrent-requests=$API_MAX_CONCURRENT $CMD_FLAGS
