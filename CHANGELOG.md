# Changelog

## Unreleased

### Fixed

- `ilo_storage_disk_capacity_byte` was 1024 times too small for drives reported via the HPE SmartStorage API (iLO 4/5).
- Processors without socket (e.g. GPUs listed as processors by iLO 6) had identical labels, so only one of them
  was reported. The `socket` label now falls back to the processor ID.
- `SIGINT` (Ctrl-C) did not stop the exporter, `SIGTERM` did not shut it down gracefully and pending traces were not flushed.
- `API_USERNAME`/`API_PASSWORD` overwrote both credentials when only one of `-api.username`/`-api.password` was set.
- The version reported by release binaries and container images is now set at build time.

### Added

- `-log.level` (`debug`, `info`, `warn`, `error`; default `info`) and `-log.format` (`logfmt`, `json`).
- `-log.redfish-bodies` logs every Redfish response body at debug level.
- `-api.timeout` limits the duration of a scrape. Scrapes are also limited by the scrape timeout sent by Prometheus
  (`X-Prometheus-Scrape-Timeout-Seconds`) minus `-web.timeout-offset` (default `500ms`) and by `-web.write-timeout`.
  When the limit is reached, pending Redfish requests are canceled and the metrics collected so far are returned.
  Previously, requests continued after Prometheus had given up on the scrape.
- `-api.insecure-skip-verify` (default `true`, as before) and `-api.ca-file` to verify iLO certificates.
- `-api.idle-conn-timeout` (default `1m`) closes idle connections to an iLO.

### Changed

- Logging is quiet by default: Redfish requests are only logged at debug level and response bodies only with
  `-log.redfish-bodies` (previously every request and response body was logged at info level).
  Failed scrapes are summarized in a single warning per scrape instead of one error per failed request.
- **Connections to an iLO are reused across scrapes** (HTTP keep-alive), avoiding a TLS handshake per request
  and per scrape. Connections are no longer leaked after a scrape.
- **`-api.max-concurrent-requests` now limits the requests per iLO across all concurrent scrapes**, e.g. when
  scraped by multiple Prometheus servers. Previously the limit applied per scrape. `0` means unlimited
  (previously `0` blocked all requests).
- HTTP proxy environment variables are still ignored and HTTP/1.1 is still used for Redfish requests.
- Logging uses `log/slog` (Prometheus `promslog` format) instead of logrus.
- The binary in release archives is named `ilo_exporter` (was `ilo5_exporter`), matching the container image.

## 1.1.3

### Fixed

- The Go version was raised to 1.26.10 for builds and releases and to 1.27.3 for the container image, fixing
  GO-2026-6809 (`crypto/x509` did not distrust compromised intermediate certificates).

### Changed

- Updated dependencies.

## 1.1.2

### Fixed

- The exporter could crash when a Redfish collection contained links to unexpected paths.
  Since the `host` parameter was not validated, this could be triggered by anyone able to reach the exporter.
- The `host` parameter is now validated to be `host[:port]`. Previously a crafted parameter could send the
  configured credentials to an arbitrary host or path. Requests without or with an invalid `host` are answered
  with `400 Bad Request` (was `500`) and are no longer logged as errors.

### Added

- `-web.allowed-targets` restricts the hosts which can be scraped to a list of host names, domain suffixes
  (`.example.com` or `*.example.com`), IP addresses and CIDR prefixes. Hosts which are not allowed are answered
  with `403 Forbidden`. Without it all hosts can be scraped as before (the configured credentials are sent to every
  requested host); a warning is logged at startup.
