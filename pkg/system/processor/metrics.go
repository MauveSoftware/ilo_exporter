// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package processor

import (
	"strings"
	"sync"

	"github.com/MauveSoftware/ilo4_exporter/pkg/client"
	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
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

// Describe describes all metrics for the processor package
func Describe(ch chan<- *prometheus.Desc) {
	ch <- countDesc
	ch <- coresDesc
	ch <- threadsDesc
	ch <- healthyDesc
}

// Collect collects processor metrics
func Collect(parentPath string, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	p := parentPath + "/Processors"
	procs := common.ResourceLinks{}

	err := cl.Get(p, &procs)
	if err != nil {
		errCh <- errors.Wrap(err, "could not get processor summary")
		return
	}

	ch <- prometheus.MustNewConstMetric(countDesc, prometheus.GaugeValue, float64(len(procs.Links.Members)), cl.HostName())

	wg.Add(len(procs.Links.Members))

	for _, l := range procs.Links.Members {
		go collectForProcessor(l.Href, cl, ch, wg, errCh)
	}
}

func collectForProcessor(link string, cl client.Client, ch chan<- prometheus.Metric, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	i := strings.Index(link, "Systems/")
	p := link[i:]

	pr := Processor{}

	err := cl.Get(p, &pr)
	if err != nil {
		errCh <- errors.Wrapf(err, "could not get processor information from %s", link)
		return
	}

	l := []string{cl.HostName(), pr.Socket, strings.Trim(pr.Model, " ")}
	ch <- prometheus.MustNewConstMetric(coresDesc, prometheus.GaugeValue, pr.TotalCores, l...)
	ch <- prometheus.MustNewConstMetric(threadsDesc, prometheus.GaugeValue, pr.TotalThreads, l...)
	ch <- prometheus.MustNewConstMetric(healthyDesc, prometheus.GaugeValue, pr.Status.HealthValue(), l...)
}
