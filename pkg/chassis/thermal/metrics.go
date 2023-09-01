// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package thermal

import (
	"context"
	"fmt"

	"github.com/MauveSoftware/ilo5_exporter/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	prefix = "ilo_chassis_"
)

var (
	fanHealthyDesc            *prometheus.Desc
	fanEnabledDesc            *prometheus.Desc
	fanCurrentDesc            *prometheus.Desc
	tempCurrentDesc           *prometheus.Desc
	tempCriticalThresholdDesc *prometheus.Desc
	tempFatalThresholdDesc    *prometheus.Desc
	tempHealthyDesc           *prometheus.Desc
)

func init() {
	l := []string{"host", "name"}

	fanHealthyDesc = prometheus.NewDesc(prefix+"fan_healthy", "Health status of the fan", l, nil)
	fanEnabledDesc = prometheus.NewDesc(prefix+"fan_enabled", "Status of the fan", l, nil)
	fanCurrentDesc = prometheus.NewDesc(prefix+"fan_current_percent", "Current power in percent", l, nil)
	tempCurrentDesc = prometheus.NewDesc(prefix+"temperature_current", "Current temperature in degree celsius", l, nil)
	tempCriticalThresholdDesc = prometheus.NewDesc(prefix+"temperature_critical", "Critcal temperature threshold in degree celsius", l, nil)
	tempFatalThresholdDesc = prometheus.NewDesc(prefix+"temperature_fatal", "Fatal temperature threshold in degree celsius", l, nil)
	tempHealthyDesc = prometheus.NewDesc(prefix+"temperature_healthy", "Health status of the temperature sensor", l, nil)
}

func Describe(ch chan<- *prometheus.Desc) {
	ch <- fanHealthyDesc
	ch <- fanEnabledDesc
	ch <- fanCurrentDesc
	ch <- tempCurrentDesc
	ch <- tempCriticalThresholdDesc
	ch <- tempFatalThresholdDesc
	ch <- tempHealthyDesc
}

func Collect(ctx context.Context, parentPath string, cc *common.CollectorContext) {
	ctx, span := cc.Tracer().Start(ctx, "Thermal.Collect", trace.WithAttributes(
		attribute.String("parent_path", parentPath),
	))
	defer span.End()

	th := Thermal{}
	err := cc.Client().Get(ctx, parentPath+"/Thermal", &th)
	if err != nil {
		cc.HandleError(fmt.Errorf("could not get thermal data: %w", err), span)
	}

	hostname := cc.Client().HostName()
	for _, f := range th.Fans {
		if f.Status.State == "UnavailableOffline" || f.Status.State == "Offline" {
			continue
		}

		collectForFan(hostname, &f, cc)
	}

	for _, t := range th.Temperatures {
		if t.Status.State == "Absent" || t.Status.State == "Offline" {
			continue
		}

		collectForTemperature(hostname, &t, cc)
	}
}

func collectForFan(hostName string, f *Fan, cc *common.CollectorContext) {
	l := []string{hostName, f.Name()}
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(fanHealthyDesc, prometheus.GaugeValue, f.Status.HealthValue(), l...),
		prometheus.MustNewConstMetric(fanEnabledDesc, prometheus.GaugeValue, f.Status.EnabledValue(), l...),
		prometheus.MustNewConstMetric(fanCurrentDesc, prometheus.GaugeValue, f.Reading(), l...),
	)
}

func collectForTemperature(hostName string, t *Temperature, cc *common.CollectorContext) {
	l := []string{hostName, t.Name}
	cc.RecordMetrics(
		prometheus.MustNewConstMetric(tempCurrentDesc, prometheus.GaugeValue, t.ReadingCelsius, l...),
		prometheus.MustNewConstMetric(tempCriticalThresholdDesc, prometheus.GaugeValue, t.UpperThresholdCritical, l...),
		prometheus.MustNewConstMetric(tempFatalThresholdDesc, prometheus.GaugeValue, t.UpperThresholdFatal, l...),
		prometheus.MustNewConstMetric(tempHealthyDesc, prometheus.GaugeValue, t.Status.HealthValue(), l...),
	)
}
