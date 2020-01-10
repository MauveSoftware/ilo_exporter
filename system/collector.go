package system

import (
	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/MauveSoftware/ilo4_exporter/system/memory"
	"github.com/MauveSoftware/ilo4_exporter/system/processor"
	"github.com/MauveSoftware/ilo4_exporter/system/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	prefix = "ilo4_"
)

var (
	powerUpDesc = prometheus.NewDesc(prefix+"power_up", "Power status", []string{"host"}, nil)
)

// NewCollector returns a new collector for system metrics
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
	ch <- powerUpDesc
	memory.Describe(ch)
	processor.Describe(ch)
	storage.Describe(ch)
}

// Collect implements prometheus.Collector interface
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	p := "Systems/1"

	s := System{}
	err := c.cl.Get(p, &s)
	if err != nil {
		logrus.Error(err)
	}

	ch <- prometheus.MustNewConstMetric(powerUpDesc, prometheus.GaugeValue, s.PowerUpValue(), c.cl.HostName())

	err = memory.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}

	err = processor.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}

	err = storage.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}
}
