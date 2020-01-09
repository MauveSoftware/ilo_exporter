package memory

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	prefix = "ilo4_memory_"
)

var (
	healthyDesc     *prometheus.Desc
	totalMemory     *prometheus.Desc
	dimmHealthyDesc *prometheus.Desc
	dimmSizeDesc    *prometheus.Desc
)

func init() {
	l := []string{"host"}
	healthyDesc = prometheus.NewDesc(prefix+"healthy", "Health status of the memory", l, nil)
	totalMemory = prometheus.NewDesc(prefix+"total_byte", "Total memory installed in bytes", l, nil)

	l = append(l, "name")
	dimmHealthyDesc = prometheus.NewDesc(prefix+"dimm_healthy", "Health status of processor", l, nil)
	dimmSizeDesc = prometheus.NewDesc(prefix+"dimm_byte", "DIMM size in bytes", l, nil)
}

func Describe(ch chan<- *prometheus.Desc) {
	ch <- healthyDesc
	ch <- totalMemory
	ch <- dimmHealthyDesc
	ch <- dimmSizeDesc
}

func Collect(parentPath string, cl client.Client, ch chan<- prometheus.Metric) error {
	p := parentPath + "/Memory"
	m := Memory{}

	err := cl.Get(p, &m)
	if err != nil {
		return errors.Wrap(err, "could not get memory summary")
	}

	var healthy float64
	if m.Status.HealthRollUp == "Ok" {
		healthy = 1
	}

	ch <- prometheus.MustNewConstMetric(healthyDesc, prometheus.GaugeValue, healthy, cl.HostName())
	ch <- prometheus.MustNewConstMetric(totalMemory, prometheus.GaugeValue, float64(m.TotalSystemMemoryGiB<<30), cl.HostName())

	for _, l := range m.Links.Members {
		err := collectForDIMM(l.Href, cl, ch)
		if err != nil {
			return errors.Wrapf(err, "could not get memory information from %s", l)
		}
	}

	return nil
}

func collectForDIMM(link string, cl client.Client, ch chan<- prometheus.Metric) error {
	i := strings.Index(link, "Systems/")
	p := link[i:]

	d := MemoryDIMM{}

	err := cl.Get(p, &d)
	if err != nil {
		return err
	}

	l := []string{cl.HostName(), d.Name}

	var healthy float64
	if d.DIMMStatus == "GoodInUse" {
		healthy = 1
	}

	ch <- prometheus.MustNewConstMetric(dimmHealthyDesc, prometheus.GaugeValue, healthy, l...)
	ch <- prometheus.MustNewConstMetric(dimmSizeDesc, prometheus.GaugeValue, float64(d.SizeMB<<20), l...)

	return nil
}
