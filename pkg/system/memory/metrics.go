// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/MauveSoftware/ilo5_exporter/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	prefix = "ilo_memory_"
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
func Collect(parentPath string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	ctx, span := cc.Tracer().Start(cc.RootCtx(), "Memory.Collect", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	p := parentPath + "/Memory"
	mem := common.MemberList{}

	err := cc.Client().Get(ctx, p, &mem)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get memory summary: %w", err), span)
		return
	}

	cc.WaitGroup().Add(len(mem.Members))

	for _, m := range mem.Members {
		go collectForDIMM(ctx, m.Path, cc)
	}
}

func collectForDIMM(ctx context.Context, link string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	ctx, span := cc.Tracer().Start(ctx, "Memory.CollectForDIMM", trace.WithAttributes(
		attribute.String("path", link),
	))
	defer span.End()

	i := strings.Index(link, "Systems/")
	p := link[i:]

	d := MemoryDIMM{}
	err := cc.Client().Get(ctx, p, &d)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get memory information from %s: %w", link, err), span)
		return
	}

	l := []string{cc.Client().HostName(), d.Name}

	if !d.IsValid() {
		return
	}

	cc.RecordMetrics(
		prometheus.MustNewConstMetric(dimmHealthyDesc, prometheus.GaugeValue, d.HealthValue(), l...),
		prometheus.MustNewConstMetric(dimmSizeDesc, prometheus.GaugeValue, float64(d.SizeMB()<<20), l...),
	)
}
