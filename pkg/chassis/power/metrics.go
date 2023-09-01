// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package power

import (
	"context"
	"fmt"

	"github.com/MauveSoftware/ilo_exporter/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	prefix = "ilo_power_"
)

var (
	powerCurrentDesc       *prometheus.Desc
	powerAvgDesc           *prometheus.Desc
	powerMinDesc           *prometheus.Desc
	powerMaxDesc           *prometheus.Desc
	powerSupplyHealthyDesc *prometheus.Desc
	powerSupplyEnabledDesc *prometheus.Desc
	powerCapacityDesc      *prometheus.Desc
)

func init() {
	l := []string{"host"}
	lpm := append(l, "id")
	powerCurrentDesc = prometheus.NewDesc(prefix+"current_watt", "Current power consumption in watt", lpm, nil)
	powerAvgDesc = prometheus.NewDesc(prefix+"average_watt", "Average power consumption in watt", lpm, nil)
	powerMinDesc = prometheus.NewDesc(prefix+"min_watt", "Minimum power consumption in watt", lpm, nil)
	powerMaxDesc = prometheus.NewDesc(prefix+"max_watt", "Maximum power consumption in watt", lpm, nil)
	powerCapacityDesc = prometheus.NewDesc(prefix+"capacity_watt", "Power capacity in watt", lpm, nil)

	l = append(l, "serial")
	powerSupplyHealthyDesc = prometheus.NewDesc(prefix+"supply_healthy", "Health status of the power supply", l, nil)
	powerSupplyEnabledDesc = prometheus.NewDesc(prefix+"supply_enabled", "Status of the power supply", l, nil)
}

func Describe(ch chan<- *prometheus.Desc) {
	ch <- powerCurrentDesc
	ch <- powerAvgDesc
	ch <- powerMinDesc
	ch <- powerMaxDesc
	ch <- powerCapacityDesc
	ch <- powerSupplyHealthyDesc
	ch <- powerSupplyEnabledDesc
}

func Collect(ctx context.Context, parentPath string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Power.Collect", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	pwr := Power{}
	err := cc.Client().Get(ctx, parentPath+"/Power", &pwr)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get power data: %w", err), span)
	}

	l := []string{cc.Client().HostName()}

	for _, pwc := range pwr.PowerControl {
		la := append(l, pwc.ID)
		cc.RecordMetrics(
			prometheus.MustNewConstMetric(powerCurrentDesc, prometheus.GaugeValue, pwc.PowerConsumedWatts, la...),
			prometheus.MustNewConstMetric(powerAvgDesc, prometheus.GaugeValue, pwc.Metrics.AverageConsumedWatts, la...),
			prometheus.MustNewConstMetric(powerMinDesc, prometheus.GaugeValue, pwc.Metrics.MinConsumedWatts, la...),
			prometheus.MustNewConstMetric(powerMaxDesc, prometheus.GaugeValue, pwc.Metrics.MaxConsumedWatts, la...),
			prometheus.MustNewConstMetric(powerCapacityDesc, prometheus.GaugeValue, pwc.PowerCapacityWatts, la...),
		)
	}

	for _, sup := range pwr.PowerSupplies {
		if sup.Status.State == "Absent" {
			continue
		}

		la := append(l, sup.SerialNumber)
		cc.RecordMetrics(
			prometheus.MustNewConstMetric(powerSupplyEnabledDesc, prometheus.GaugeValue, sup.Status.EnabledValue(), la...),
			prometheus.MustNewConstMetric(powerSupplyHealthyDesc, prometheus.GaugeValue, sup.Status.HealthValue(), la...),
		)
	}
}
