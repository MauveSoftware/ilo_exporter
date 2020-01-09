package chassis

import (
	"github.com/MauveSoftware/ilo4_exporter/chassis/power"
	"github.com/MauveSoftware/ilo4_exporter/chassis/thermal"
	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
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
	power.Describe(ch)
	thermal.Describe(ch)
}

// Collect implements prometheus.Collector interface
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	p := "Chassis/1"
	err := power.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}

	err = thermal.Collect(p, c.cl, ch)
	if err != nil {
		logrus.Error(err)
	}
}
