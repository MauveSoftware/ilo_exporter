package power

import (
	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
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

func Collect(parentPath string, cl client.Client, ch chan<- prometheus.Metric) error {
	pwr := Power{}

	err := cl.Get(parentPath+"/Power", &pwr)
	if err != nil {
		return errors.Wrap(err, "could not get power data")
	}

	l := []string{cl.HostName()}
	ch <- prometheus.MustNewConstMetric(powerCurrentDesc, prometheus.GaugeValue, pwr.PowerConsumedWatts, l...)
	ch <- prometheus.MustNewConstMetric(powerAvgDesc, prometheus.GaugeValue, pwr.Metrics.AverageConsumedWatts, l...)
	ch <- prometheus.MustNewConstMetric(powerMinDesc, prometheus.GaugeValue, pwr.Metrics.MinConsumedWatts, l...)
	ch <- prometheus.MustNewConstMetric(powerMaxDesc, prometheus.GaugeValue, pwr.Metrics.MaxConsumedWatts, l...)

	for _, sup := range pwr.PowerSupplies {
		la := append(l, sup.SerialNumber)
		ch <- prometheus.MustNewConstMetric(powerSupplyEnabledDesc, prometheus.GaugeValue, sup.Status.EnabledValue(), la...)
		ch <- prometheus.MustNewConstMetric(powerSupplyHealthyDesc, prometheus.GaugeValue, sup.Status.HealthValue(), la...)
	}

	return nil
}
