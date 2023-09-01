// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package system

import (
	"context"
	"time"

	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/MauveSoftware/ilo_exporter/pkg/common"
	"github.com/MauveSoftware/ilo_exporter/pkg/system/memory"
	"github.com/MauveSoftware/ilo_exporter/pkg/system/processor"
	"github.com/MauveSoftware/ilo_exporter/pkg/system/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

const (
	prefix = "ilo_"
)

var (
	powerUpDesc        = prometheus.NewDesc(prefix+"power_up", "Power status", []string{"host"}, nil)
	scrapeDurationDesc = prometheus.NewDesc(prefix+"system_scrape_duration_second", "Scrape duration for the system module", []string{"host"}, nil)
)

// NewCollector returns a new collector for system metrics
func NewCollector(ctx context.Context, cl client.Client, tracer trace.Tracer) prometheus.Collector {
	return &collector{
		rootCtx: ctx,
		cl:      cl,
		tracer:  tracer,
	}
}

type collector struct {
	rootCtx context.Context
	cl      client.Client
	tracer  trace.Tracer
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

	ctx, span := c.tracer.Start(c.rootCtx, "System.Collect")
	defer span.End()

	p := "Systems/1"

	s := System{}
	err := c.cl.Get(ctx, p, &s)
	if err != nil {
		logrus.Error(err)
	}

	ch <- prometheus.MustNewConstMetric(powerUpDesc, prometheus.GaugeValue, s.PowerUpValue(), c.cl.HostName())

	doneCh := make(chan interface{})

	cc := common.NewCollectorContext(ctx, c.cl, ch, c.tracer)

	cc.WaitGroup().Add(3)
	go func() {
		cc.WaitGroup().Wait()
		doneCh <- nil
	}()

	go memory.Collect(p, cc)
	go processor.Collect(p, cc)
	go storage.Collect(p, cc)

	<-doneCh
	if cc.ErrCount() == 0 {
		duration := time.Since(start).Seconds()
		ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration, c.cl.HostName())
	}
}
