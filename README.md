[![Go Report Card](https://goreportcard.com/badge/github.com/mauvesoftware/ilo_exporter)](https://goreportcard.com/report/github.com/mauvesoftware/ilo_exporter)
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
docker run -d --restart always --name ilo_exporter -p 19545:19545 -e API_USERNAME=ilo_exporter -e API_PASSWORD=g3tM3trics mauvesoftware/ilo_exporter
```

## Prometheus configuration
To get metrics for 172.16.0.200 using https://my-exporter-tld/metrics?hosts=172.16.0.200

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

## License
(c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.

## Prometheus
see https://prometheus.io/
