// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
)

const (
	prefix = "ilo4_memory_"
)

var (
	totalMemory     *prometheus.Desc
	dimmHealthyDesc *prometheus.Desc
	dimmSizeDesc    *prometheus.Desc
)

func init() {
	l := []string{"host"}
	totalMemory = prometheus.NewDesc(prefix+"total_byte", "Total memory installed in bytes", l, nil)

	l = append(l, "name")
	dimmHealthyDesc = prometheus.NewDesc(prefix+"dimm_healthy", "Health status of processor", l, nil)
	dimmSizeDesc = prometheus.NewDesc(prefix+"dimm_byte", "DIMM size in bytes", l, nil)
}

// Describe describes all metrics for the memory package
func Describe(ch chan<- *prometheus.Desc) {
	ch <- totalMemory
	ch <- dimmHealthyDesc
	ch <- dimmSizeDesc
}

// Collect collects metrics for memory modules
func Collect(systemPath string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	ctx, span := cc.Tracer().Start(cc.RootCtx(), "Memory.Collect", trace.WithAttributes(
		attribute.String("parent_path", systemPath),
	))
	defer span.End()

	m := Memory{}
	err := cc.Client().Get(ctx, systemPath, &m)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get memory summary: %w", err), span)
		return
	}

	hostname := cc.Client().HostName()
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(totalMemory, prometheus.GaugeValue, float64(m.MemorySummary.TotalSystemMemoryGiB<<30), hostname),
	)

	collectForDIMMs(ctx, systemPath, cc)
}

func collectForDIMMs(ctx context.Context, parentPath string, cc *common.CollectorContext) {
	p := parentPath + "/Memory"

	ctx, span := cc.Tracer().Start(ctx, "Memory.CollectForDIMMs", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	mem := common.ResourceLinks{}
	err := cc.Client().Get(ctx, p, &mem)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get DIMM list: %w", err), span)
		return
	}

	cc.WaitGroup().Add(len(mem.Links.Members))

	for _, l := range mem.Links.Members {
		go collectForDIMM(ctx, l.Href, cc)
	}
}

func collectForDIMM(ctx context.Context, link string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	i := strings.Index(link, "Systems/")
	p := link[i:]

	ctx, span := cc.Tracer().Start(ctx, "Memory.CollectForDIMM", trace.WithAttributes(
		attribute.String("path", link),
	))
	defer span.End()

	d := MemoryDIMM{}
	err := cc.Client().Get(ctx, p, &d)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get memory information from %s: %w", link, err), span)
		return
	}

	l := []string{cc.Client().HostName(), d.Name}

	if d.DIMMStatus == "Unknown" {
		return
	}

	var healthy float64
	if d.DIMMStatus == "GoodInUse" {
		healthy = 1
	}

	cc.RecordMetrics(
		prometheus.MustNewConstMetric(dimmHealthyDesc, prometheus.GaugeValue, healthy, l...),
		prometheus.MustNewConstMetric(dimmSizeDesc, prometheus.GaugeValue, float64(d.SizeMB<<20), l...),
	)
}
