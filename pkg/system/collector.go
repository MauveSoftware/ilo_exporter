// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package system

import (
	"sync"
	"time"

	"github.com/MauveSoftware/ilo4_exporter/pkg/client"
	"github.com/MauveSoftware/ilo4_exporter/pkg/system/memory"
	"github.com/MauveSoftware/ilo4_exporter/pkg/system/processor"
	"github.com/MauveSoftware/ilo4_exporter/pkg/system/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	prefix = "ilo4_"
)

var (
	powerUpDesc        = prometheus.NewDesc(prefix+"power_up", "Power status", []string{"host"}, nil)
	scrapeDurationDesc = prometheus.NewDesc(prefix+"system_scrape_duration_second", "Scrape duration for the system module", []string{"host"}, nil)
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
	ch <- scrapeDurationDesc
	memory.Describe(ch)
	processor.Describe(ch)
	storage.Describe(ch)
}

// Collect implements prometheus.Collector interface
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	p := "Systems/1"

	s := System{}
	err := c.cl.Get(p, &s)
	if err != nil {
		logrus.Error(err)
	}

	ch <- prometheus.MustNewConstMetric(powerUpDesc, prometheus.GaugeValue, s.PowerUpValue(), c.cl.HostName())

	wg := &sync.WaitGroup{}
	doneCh := make(chan interface{})
	errCh := make(chan error)

	wg.Add(3)

	go func() {
		wg.Wait()
		doneCh <- nil
	}()

	go memory.Collect(p, c.cl, ch, wg, errCh)
	go processor.Collect(p, c.cl, ch, wg, errCh)
	go storage.Collect(p, c.cl, ch, wg, errCh)

	errs := 0
	for {
		select {
		case <-doneCh:
			if errs == 0 {
				duration := time.Now().Sub(start).Seconds()
				ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration, c.cl.HostName())
			}

			return
		case err = <-errCh:
			errs++
			logrus.Error(err)
		}
	}
}
