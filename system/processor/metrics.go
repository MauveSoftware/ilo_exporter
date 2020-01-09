package processor

import (
	"strings"

	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	prefix = "ilo4_processor_"
)

var (
	countDesc   *prometheus.Desc
	coresDesc   *prometheus.Desc
	threadsDesc *prometheus.Desc
	healthyDesc *prometheus.Desc
)

func init() {
	l := []string{"host"}
	countDesc = prometheus.NewDesc(prefix+"count", "Number of processors", l, nil)

	l = append(l, "socket", "model")
	coresDesc = prometheus.NewDesc(prefix+"core_count", "Number of cores of processor", l, nil)
	threadsDesc = prometheus.NewDesc(prefix+"thread_count", "Number of threads of processor", l, nil)
	healthyDesc = prometheus.NewDesc(prefix+"healthy", "Health status of processor", l, nil)
}

func Describe(ch chan<- *prometheus.Desc) {
	ch <- countDesc
	ch <- coresDesc
	ch <- threadsDesc
	ch <- healthyDesc
}

func Collect(parentPath string, cl client.Client, ch chan<- prometheus.Metric) error {
	p := parentPath + "/Processors"
	procs := Processors{}

	err := cl.Get(p, &procs)
	if err != nil {
		return errors.Wrap(err, "could not get processor summary")
	}

	ch <- prometheus.MustNewConstMetric(countDesc, prometheus.GaugeValue, procs.Count, cl.HostName())

	for _, l := range procs.Links.Members {
		err := collectForProcessor(l.Href, cl, ch)
		if err != nil {
			return errors.Wrapf(err, "could not get processor information from %s", l)
		}
	}

	return nil
}

func collectForProcessor(link string, cl client.Client, ch chan<- prometheus.Metric) error {
	i := strings.Index(link, "Systems/")
	p := link[i:]

	pr := Processor{}

	err := cl.Get(p, &pr)
	if err != nil {
		return err
	}

	l := []string{cl.HostName(), pr.Socket, strings.Trim(pr.Model, " ")}
	ch <- prometheus.MustNewConstMetric(coresDesc, prometheus.GaugeValue, pr.TotalCores, l...)
	ch <- prometheus.MustNewConstMetric(threadsDesc, prometheus.GaugeValue, pr.TotalThreads, l...)
	ch <- prometheus.MustNewConstMetric(healthyDesc, prometheus.GaugeValue, pr.Status.HealthValue(), l...)

	return nil
}
