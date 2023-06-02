// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package power

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
)

const (
	prefix = "ilo4_power_"
)

var (
	powerCurrentDesc       *prometheus.Desc
	powerAvgDesc           *prometheus.Desc
	powerMinDesc           *prometheus.Desc
	powerMaxDesc           *prometheus.Desc
	powerSupplyHealthyDesc *prometheus.Desc
	powerSupplyEnabledDesc *prometheus.Desc
)

func init() {
	l := []string{"host"}
	powerCurrentDesc = prometheus.NewDesc(prefix+"current_watt", "Current power consumption in watt", l, nil)
	powerAvgDesc = prometheus.NewDesc(prefix+"average_watt", "Average power consumption in watt", l, nil)
	powerMinDesc = prometheus.NewDesc(prefix+"min_watt", "Minimum power consumption in watt", l, nil)
	powerMaxDesc = prometheus.NewDesc(prefix+"max_watt", "Maximum power consumption in watt", l, nil)

	l = append(l, "serial")
	powerSupplyHealthyDesc = prometheus.NewDesc(prefix+"supply_healthy", "Health status of the power supply", l, nil)
	powerSupplyEnabledDesc = prometheus.NewDesc(prefix+"supply_enabled", "Status of the power supply", l, nil)
}

func Describe(ch chan<- *prometheus.Desc) {
	ch <- powerCurrentDesc
	ch <- powerAvgDesc
	ch <- powerMinDesc
	ch <- powerMaxDesc
	ch <- powerSupplyHealthyDesc
	ch <- powerSupplyEnabledDesc
}

func Collect(ctx context.Context, parentPath string, cc *common.CollectorContext) error {
	ctx, span := cc.Tracer().Start(ctx, "Power.Collect", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	pwr := Power{}
	err := cc.Client().Get(ctx, parentPath+"/Power", &pwr)
	if err != nil {
		return errors.Wrap(err, "could not get power data")
	}

	l := []string{cc.Client().HostName()}
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(powerCurrentDesc, prometheus.GaugeValue, pwr.PowerConsumedWatts, l...),
		prometheus.MustNewConstMetric(powerAvgDesc, prometheus.GaugeValue, pwr.Metrics.AverageConsumedWatts, l...),
		prometheus.MustNewConstMetric(powerMinDesc, prometheus.GaugeValue, pwr.Metrics.MinConsumedWatts, l...),
		prometheus.MustNewConstMetric(powerMaxDesc, prometheus.GaugeValue, pwr.Metrics.MaxConsumedWatts, l...),
	)

	for _, sup := range pwr.PowerSupplies {
		collectForPowerSupply(sup, l, cc)
	}

	return nil
}

func collectForPowerSupply(sup PowerSupply, labelVals []string, cc *common.CollectorContext) {
	if sup.Status.State == "Absent" {
		return
	}

	la := append(labelVals, sup.SerialNumber)
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(powerSupplyEnabledDesc, prometheus.GaugeValue, sup.Status.EnabledValue(), la...),
		prometheus.MustNewConstMetric(powerSupplyHealthyDesc, prometheus.GaugeValue, sup.Status.HealthValue(), la...),
	)
}
