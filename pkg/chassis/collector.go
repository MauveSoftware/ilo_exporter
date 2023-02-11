// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package chassis

import (
	"context"
	"time"

	"github.com/MauveSoftware/ilo4_exporter/pkg/chassis/power"
	"github.com/MauveSoftware/ilo4_exporter/pkg/chassis/thermal"
	"github.com/MauveSoftware/ilo4_exporter/pkg/client"
	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

const (
	prefix = "ilo4_"
)

var (
	scrapeDurationDesc = prometheus.NewDesc(prefix+"chassis_scrape_duration_second", "Scrape duration for the chassis module", []string{"host"}, nil)
)

// NewCollector returns a new collector for chassis metrics
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
	ch <- scrapeDurationDesc
	power.Describe(ch)
	thermal.Describe(ch)
}

// Collect implements prometheus.Collector interface
func (c *collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	ctx, span := c.tracer.Start(c.rootCtx, "Chassis.Collect")
	defer span.End()

	p := "Chassis/1"

	cc := common.NewCollectorContext(ctx, c.cl, ch, c.tracer)
	power.Collect(ctx, p, cc)
	thermal.Collect(ctx, p, cc)

	duration := time.Since(start).Seconds()
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration, c.cl.HostName())
}
