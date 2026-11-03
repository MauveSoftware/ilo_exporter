FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY *.go ./
COPY pkg ./pkg

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
  GOARM=${TARGETVARIANT#v} \
  go build -trimpath \
  -ldflags="-s -w" \
  -o /out/ilo_exporter .


FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

WORKDIR /app
COPY --from=builder /out/ilo_exporter /app/ilo_exporter

EXPOSE 9545
ENTRYPOINT ["/app/ilo_exporter"]
