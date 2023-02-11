// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package processor

import (
	"context"
	"fmt"
	"strings"

	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
func Collect(parentPath string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	ctx, span := cc.Tracer().Start(cc.RootCtx(), "Processor.Collect", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	p := parentPath + "/Processors"
	procs := common.ResourceLinks{}

	err := cc.Client().Get(ctx, p, &procs)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get processor summary: %w", err), span)
		return
	}

	cc.RecordMetrics(
		prometheus.MustNewConstMetric(countDesc, prometheus.GaugeValue, float64(len(procs.Links.Members)), cc.Client().HostName()),
	)

	cc.WaitGroup().Add(len(procs.Links.Members))

	for _, l := range procs.Links.Members {
		go collectForProcessor(ctx, l.Href, cc)
	}
}

func collectForProcessor(ctx context.Context, link string, cc *common.CollectorContext) {
	defer cc.WaitGroup().Done()

	ctx, span := cc.Tracer().Start(ctx, "Storage.CollectProcessor", trace.WithAttributes(
		attribute.String("path", link),
	))
	defer span.End()

	i := strings.Index(link, "Systems/")
	p := link[i:]

	pr := Processor{}

	err := cc.Client().Get(ctx, p, &pr)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get processor information from %s: %w", link, err), span)
		return
	}

	l := []string{cc.Client().HostName(), pr.Socket, strings.Trim(pr.Model, " ")}
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(coresDesc, prometheus.GaugeValue, pr.TotalCores, l...),
		prometheus.MustNewConstMetric(threadsDesc, prometheus.GaugeValue, pr.TotalThreads, l...),
		prometheus.MustNewConstMetric(healthyDesc, prometheus.GaugeValue, pr.Status.HealthValue(), l...),
	)
}
