package memory

import (
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/MauveSoftware/ilo4_exporter/common"
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

// Describe describes all metrics for the memory package
func Describe(ch chan<- *prometheus.Desc) {
	ch <- healthyDesc
	ch <- totalMemory
	ch <- dimmHealthyDesc
	ch <- dimmSizeDesc
}

// Collect collects metrics for memory modules
func Collect(systemPath string, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	m := Memory{}

	err := cl.Get(systemPath, &m)
	if err != nil {
		errCh <- errors.Wrap(err, "could not get memory summary")
		return
	}

	var healthy float64
	if strings.ToLower(m.MemorySummary.Status.HealthRollUp) == "ok" {
		healthy = 1
	}

	ch <- prometheus.MustNewConstMetric(healthyDesc, prometheus.GaugeValue, healthy, cl.HostName())
	ch <- prometheus.MustNewConstMetric(totalMemory, prometheus.GaugeValue, float64(m.MemorySummary.TotalSystemMemoryGiB<<30), cl.HostName())

	collectForDIMMs(systemPath, cl, ch, errCh)
}

func collectForDIMMs(parentPath string, cl client.Client, ch chan<- prometheus.Metric, errCh chan<- error) {
	p := parentPath + "/Memory"

	mem := common.ResourceLinks{}
	err := cl.Get(p, &mem)
	if err != nil {
		errCh <- errors.Wrap(err, "could not get DIMM list")
		return
	}

	for _, l := range mem.Links.Members {
		collectForDIMM(l.Href, cl, ch, errCh)
	}
}

func collectForDIMM(link string, cl client.Client, ch chan<- prometheus.Metric, errCh chan<- error) {
	i := strings.Index(link, "Systems/")
	p := link[i:]

	d := MemoryDIMM{}

	err := cl.Get(p, &d)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get memory information from %s", link)
		return
	}

	l := []string{cl.HostName(), d.Name}

	var healthy float64
	if d.DIMMStatus == "GoodInUse" {
		healthy = 1
	}

	ch <- prometheus.MustNewConstMetric(dimmHealthyDesc, prometheus.GaugeValue, healthy, l...)
	ch <- prometheus.MustNewConstMetric(dimmSizeDesc, prometheus.GaugeValue, float64(d.SizeMB<<20), l...)
}
