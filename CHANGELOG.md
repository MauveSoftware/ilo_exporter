# Changelog

## Unreleased

### Fixed

- `ilo_storage_disk_capacity_byte` was 1024 times too small for drives reported via the HPE SmartStorage API (iLO 4/5).
- Processors without socket (e.g. GPUs listed as processors by iLO 6) had identical labels, so only one of them
  was reported. The `socket` label now falls back to the processor ID.
- On iLO 6 and 7, the HPE SmartStorage API (removed in iLO 6) was requested on every scrape. The failing request
  caused an error log per scrape and suppressed `ilo_system_scrape_duration_second`.
- SmartStorage drives were not collected on iLO 5 firmware returning controller links without trailing slash.
- Drives of iLO 5 Smart Array controllers reported by both the DMTF storage model and the SmartStorage API are
  only reported once (from the DMTF storage model).
- Empty DIMM slots, empty drive bays, absent fans and empty CPU sockets are no longer reported (empty DIMM slots
  were reported with `ilo_memory_dimm_byte` 0 and `ilo_memory_dimm_healthy` 1). `ilo_processor_count` does not
  count empty sockets.
- `ilo_power_up` (0) and `ilo_system_info` (with empty labels) were reported when the system resource could not be
  retrieved, e.g. for unreachable hosts. They are now omitted, `ilo_up` is 0 instead.
- `ilo_memory_healthy` and `ilo_memory_total_byte` were described but never reported. They are now reported based
  on the memory summary of the system resource.
- `SIGINT` (Ctrl-C) did not stop the exporter, `SIGTERM` did not shut it down gracefully and pending traces were not flushed.
- `API_USERNAME`/`API_PASSWORD` overwrote both credentials when only one of `-api.username`/`-api.password` was set.
- The version reported by release binaries and container images is now set at build time.

### Added

- New metrics:
  - `ilo_up`: 1 if the system resource of the iLO could be retrieved, 0 otherwise.
  - `ilo_scrape_collector_success{collector}` and `ilo_scrape_collector_duration_seconds{collector}` per collector
    (`system`, `manager`, `power`, `thermal`, `processor`, `storage`, `memory`).
  - `ilo_scrape_redfish_requests`: number of Redfish requests sent during the scrape, to monitor the load on the iLO.
- The `collect[]` query parameter limits a scrape to the given collectors, e.g.
  `/metrics?host=ilo1&collect[]=power&collect[]=thermal`. This allows scraping inventory (`memory`, `processor`,
  `storage`) less often than health and sensor data. The system resource is always collected.
- When the iLO is unreachable or rejects the credentials, no further requests are sent after the system resource.
  Previously every collector tried, causing several failed logins per scrape.
- `-log.level` (`debug`, `info`, `warn`, `error`; default `info`) and `-log.format` (`logfmt`, `json`).
- `-log.redfish-bodies` logs every Redfish response body at debug level.
- `-api.timeout` limits the duration of a scrape. Scrapes are also limited by the scrape timeout sent by Prometheus
  (`X-Prometheus-Scrape-Timeout-Seconds`) minus `-web.timeout-offset` (default `500ms`) and by `-web.write-timeout`.
  When the limit is reached, pending Redfish requests are canceled and the metrics collected so far are returned.
  Previously, requests continued after Prometheus had given up on the scrape.
- `-api.insecure-skip-verify` (default `true`, as before) and `-api.ca-file` to verify iLO certificates.
- `-api.idle-conn-timeout` (default `1m`) closes idle connections to an iLO.
- `-api.discovery-ttl` (default `1h`) and `-api.cache-max-hosts` (default `4096`): the capabilities and resource
  layout of an iLO are discovered once and cached (4 requests per iLO and hour). The cache is invalidated when a
  resource is not found, the firmware version changes or the IP address points to another system.
- **Collections (memory, processors, drives) are retrieved with a single request using `$expand=.`** if supported
  by the iLO (iLO 5, 6 and 7). This reduces the number of requests per scrape from about 45 to about 10 for a
  typical server. `-api.expand=false` disables it. Paginated collections (`Members@odata.nextLink`) are supported.
- Support for iLOs without the deprecated `Power` and `Thermal` resources of the chassis: fans, temperatures, power
  supplies and power (capacity and current consumption) are collected from `ThermalSubsystem`, `Sensors`,
  `PowerSubsystem` and `EnvironmentMetrics` instead, using the same metric names and labels. Average, minimum and
  maximum power consumption are not available from these resources. As long as an iLO provides the deprecated
  resources (iLO 5, 6 and 7 up to at least v1.25), they are used since they contain all metrics in two requests.

### Changed

- Logging is quiet by default: Redfish requests are only logged at debug level and response bodies only with
  `-log.redfish-bodies` (previously every request and response body was logged at info level).
  Instead of one error per failed request, a warning is logged when an iLO goes down or a scrape becomes
  incomplete and an info message when it recovers.
- Resources are located by following the links of the Redfish service instead of using hard coded paths
  (`Systems/1`, `Chassis/1`, `Managers/1` remain the fallback).
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
