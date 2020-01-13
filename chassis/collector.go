package chassis

import (
	"time"

	"github.com/MauveSoftware/ilo4_exporter/chassis/power"
	"github.com/MauveSoftware/ilo4_exporter/chassis/thermal"
	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	prefix = "ilo4_"
)

var (
	scrapeDurationDesc = prometheus.NewDesc(prefix+"chassis_scrape_duration_second", "Scrape duration for the chassis module", []string{"host"}, nil)
)

// NewCollector returns a new collector for chassis metrics
func NewCollector(cl client.Client) prometheus.Collector {
	return &collector{
		cl: cl,
	}
}

type collector struct {
	cl client.Client
}

// Describe implements prometheus.Collector interface
func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	power.Describe(ch)
	thermal.Describe(ch)
}

// Collect implements prometheus.Collector interface
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	p := "Chassis/1"
	err := power.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}

	err = thermal.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}

	duration := time.Now().Sub(start).Seconds()
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration, c.cl.HostName())
}
