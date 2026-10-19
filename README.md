# ilo_exporter
Metrics exporter for HP iLO to prometheus

## Breaking changes
Beginning with version 1.0.0 the projects ilo4_exporter and ilo5_exporter were merged to ilo_exporter. Due to this change metric names are based on ilo5_exporter but were renamed to show compatiblity to both versions.

## Install
```
go get -u github.com/MauveSoftware/ilo_exporter
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

## Prometheus configuration
To get metrics for 172.16.0.200 using https://my-exporter-tld/metrics?host=172.16.0.200

```bash
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
* The `host` parameter must be `host[:port]`. Other values are rejected with `400 Bad Request`, hosts which are not
  allowed with `403 Forbidden`.

## Grafana

For users of [Grafana](https://grafana.com/), this repository includes an example [dashboard](iLO-grafana-dashboard.json) and example [alert rules](ilo-grafana-alerts.yaml).

## License
(c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.

## Prometheus
see https://prometheus.io/
