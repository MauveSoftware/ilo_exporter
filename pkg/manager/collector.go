// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package manager

import (
	"context"
	"time"

	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

const (
	prefix = "ilo_manager_"
)

var (
	infoDesc           = prometheus.NewDesc(prefix+"info", "System Info", []string{"host", "firmware_version"}, nil)
	scrapeDurationDesc = prometheus.NewDesc(prefix+"scrape_duration_second", "Scrape duration for the manager module", []string{"host"}, nil)
)

// NewCollector returns a new collector for manager metrics
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
	ch <- infoDesc
	ch <- scrapeDurationDesc
}

// Collect implements prometheus.Collector interface
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	ctx, span := c.tracer.Start(c.rootCtx, "Manager.Collect")
	defer span.End()

	p := "Managers/1"

	m := &manager{}
	err := c.cl.Get(ctx, p, &m)
	if err != nil {
		logrus.Error(err)
		return
	}

	duration := time.Since(start).Seconds()
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration, c.cl.HostName())
	ch <- prometheus.MustNewConstMetric(infoDesc, prometheus.GaugeValue, 1, c.cl.HostName(), m.FirmwareVersion)
}
