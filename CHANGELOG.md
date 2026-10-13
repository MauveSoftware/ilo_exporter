# Changelog

## Unreleased

### Fixed

- `ilo_storage_disk_capacity_byte` was 1024 times too small for drives reported via the HPE SmartStorage API (iLO 4/5).
- `SIGINT` (Ctrl-C) did not stop the exporter, `SIGTERM` did not shut it down gracefully and pending traces were not flushed.
- `API_USERNAME`/`API_PASSWORD` overwrote both credentials when only one of `-api.username`/`-api.password` was set.
- The version reported by release binaries and container images is now set at build time.

### Added

- `-log.level` (`debug`, `info`, `warn`, `error`; default `info`) and `-log.format` (`logfmt`, `json`).
- `-log.redfish-bodies` logs every Redfish response body at debug level.

### Changed

- Logging is quiet by default: Redfish requests are only logged at debug level and response bodies only with
  `-log.redfish-bodies` (previously every request and response body was logged at info level).
  Failed scrapes are summarized in a single warning per scrape instead of one error per failed request.
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
