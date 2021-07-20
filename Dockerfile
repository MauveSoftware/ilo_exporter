FROM golang as builder
ADD . /go/ilo4_exporter/
WORKDIR /go/ilo4_exporter
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/ilo4_exporter

FROM alpine:latest
ENV API_USERNAME ''
ENV API_PASSWORD ''
ENV API_MAX_CONCURRENT '4'
RUN apk --no-cache add ca-certificates bash
COPY --from=builder /go/bin/ilo4_exporter /app/ilo4_exporter
EXPOSE 9545
ENTRYPOINT /app/ilo4_exporter -api.username=$API_USERNAME -api.password=$API_PASSWORD -api.max-concurrent-requests=$API_MAX_CONCURRENT
