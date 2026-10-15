# ilo_exporter
Metrics exporter for HP iLO to prometheus

The exporter uses the Redfish API of iLO 5, 6 and 7 (iLO 4 is supported on a best effort basis).

## Breaking changes
Beginning with version 1.0.0 the projects ilo4_exporter and ilo5_exporter were merged to ilo_exporter. Due to this change metric names are based on ilo5_exporter but were renamed to show compatiblity to both versions.

See [CHANGELOG.md](CHANGELOG.md) for changes of metric values and series in newer versions.

## Install
```
go install github.com/MauveSoftware/ilo_exporter@latest
```

## Usage
Running the exporter with the following test credentials:

```
Username: ilo_exporter
Password: g3tM3trics
```

### Binary
```bash
./ilo_exporter -api.username=ilo_exporter -api.password=g3tM3trics
```

### Docker
```bash
docker run -d --restart always --name ilo_exporter -p 9545:9545 -e API_USERNAME=ilo_exporter -e API_PASSWORD=g3tM3trics mauvesoftware/ilo_exporter
```

The container image (and binary) also reads the following environment variables:

| Variable             | Description                                                          |
|----------------------|----------------------------------------------------------------------|
| `API_USERNAME`       | Username (if `-api.username` is not set)                             |
| `API_PASSWORD`       | Password (if `-api.password` is not set)                             |
| `API_MAX_CONCURRENT` | Default of `-api.max-concurrent-requests`                            |
| `CMD_FLAGS`          | Additional command line flags, e.g. `-api.auth=session -log.level=debug` |

Run `ilo_exporter -h` for all flags.

### Building the image
Pass the version to the build, so it can be checked in the running exporter (`-version`, startup log and
`ilo_exporter_build_info` metric):

```bash
VERSION=$(git describe --tags --always --dirty)
docker build --build-arg VERSION=$VERSION -t my-registry/ilo_exporter:$VERSION .
```

When running the exporter as Docker Swarm service, deploy a new image with `docker service update --image`:
the image of a service is pinned to the digest it resolved to, so `docker service update --force` restarts the
service with the same image, even if the tag was pushed again.

## Prometheus configuration
To get metrics for 172.16.0.200 using https://my-exporter-tld/metrics?host=172.16.0.200

```yaml
  - job_name: 'ilo'
    scrape_interval: 300s
    scrape_timeout: 120s
    scheme: https
    static_configs:
      - targets:
          - 172.16.0.200
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_host
      - source_labels: [__param_host]
        target_label: instance
        replacement: '${1}'
      - target_label: __address__
        replacement: my-exporter-tld
```

The exporter stops collecting when the scrape timeout sent by Prometheus (minus `-web.timeout-offset`) is reached
and returns the metrics collected so far.

### Scraping inventory less often
The `collect[]` parameter limits a scrape to the given collectors (`manager`, `power`, `thermal`, `processor`,
`storage`, `memory`). The system resource (`ilo_up`, `ilo_power_up`, `ilo_system_info`, memory summary) is always
collected. This allows scraping inventory less often than sensor data, e.g. with two jobs:

```yaml
  - job_name: 'ilo'
    scrape_interval: 60s
    params:
      collect[]: [power, thermal, manager]
    # static_configs and relabel_configs as above
  - job_name: 'ilo_inventory'
    scrape_interval: 30m
    params:
      collect[]: [processor, storage, memory]
    # static_configs and relabel_configs as above
```

## Load on the iLO
iLOs have limited resources, so the exporter tries to keep the number of requests low:

* Collections (memory, processors, drives) are retrieved with a single request using `$expand` if supported
  (`-api.expand`). A typical scrape takes about 10 requests.
* The capabilities and resource layout of an iLO are discovered once and cached (`-api.discovery-ttl`).
* Connections are reused across scrapes (`-api.idle-conn-timeout`).
* `-api.max-concurrent-requests` limits the concurrent requests per iLO, also across concurrent scrapes
  (e.g. by multiple Prometheus servers).
* If the iLO is unreachable or rejects the credentials, no further requests are sent.

The number of requests per scrape is exported as `ilo_scrape_redfish_requests`.

### Authentication
By default every request is authenticated using HTTP basic authentication, for which iLO creates and deletes a
session per request. This is especially expensive when using directory (LDAP/AD) accounts.

With `-api.auth=session` the exporter creates one Redfish session per iLO, which is renewed when it expires and deleted
on shutdown. Session authentication is also required if basic authentication is disabled on the iLO.
Note that each exporter instance uses one session per iLO. Sessions of an exporter which was not shut down gracefully
remain until they time out (30 minutes by default).

## Security
* **The credentials configured are sent to every host requested via the `host` parameter.** Without
  `-web.allowed-targets` anyone able to reach the exporter can request `?host=<a host of theirs>` and receive the
  credentials, so a warning is logged at startup. Restrict the hosts which can be scraped, e.g.
  `-web.allowed-targets=.ilo.example.com,10.0.0.0/8,ilo-01.example.net`:
  * Entries starting with a dot (or `*.`) match all hosts in that domain: `.ilo.example.com` matches
    `srv1.ilo.example.com` and `srv1.dc2.ilo.example.com`, but not `ilo.example.com` itself.
  * Host names match exactly (case insensitive).
  * IP addresses and CIDR prefixes only match targets given as IP address: host names are not resolved.

  Also restrict network access to the exporter to the Prometheus servers scraping it.
* iLO certificates are not verified by default, since iLOs typically use self signed certificates. Use
  `-api.insecure-skip-verify=false` and `-api.ca-file` to verify them.
* The `host` parameter must be `host[:port]`. Other values are rejected with `400 Bad Request`, hosts which are not
  allowed with `403 Forbidden`.

## Logging
By default only state changes are logged: a warning when an iLO goes down or a scrape is incomplete and an
info message when it recovers. `-log.level=debug` logs every Redfish request, `-log.redfish-bodies` additionally
the response bodies.

## Metrics
Besides the hardware metrics (`ilo_power_*`, `ilo_chassis_*`, `ilo_memory_*`, `ilo_processor_*`, `ilo_storage_*`,
`ilo_system_info`, `ilo_manager_info`), the following metrics describe the scrape:

| Metric                                            | Description                                                 |
|---------------------------------------------------|-------------------------------------------------------------|
| `ilo_up`                                          | 1 if the system resource of the iLO could be retrieved      |
| `ilo_scrape_collector_success{collector}`         | 1 if all resources of the collector could be retrieved      |
| `ilo_scrape_collector_duration_seconds{collector}`| Duration of the collector                                   |
| `ilo_scrape_redfish_requests`                     | Number of Redfish requests sent during the scrape           |
| `ilo_exporter_build_info{version,goversion}`      | Version of the exporter (always 1)                          |

## Grafana

For users of [Grafana](https://grafana.com/), this repository includes an example [dashboard](iLO-grafana-dashboard.json) and example [alert rules](ilo-grafana-alerts.yaml).

## Development
Tests run against a fake Redfish service (`internal/fakeilo`) serving [HPE iLO mockups](testdata/mockups).
The golden files in `testdata/golden` contain the metrics and the Redfish requests of a scrape per iLO generation.
Run `go test . -update` to update them after intended changes.

## License
(c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.

## Prometheus
see https://prometheus.io/
