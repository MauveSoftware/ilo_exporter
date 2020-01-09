package thermal

import (
	"github.com/MauveSoftware/ilo4_exporter/client"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	prefix = "ilo4_chassis_"
)

var (
	fanHealthyDesc            *prometheus.Desc
	fanEnabledDesc            *prometheus.Desc
	fanCurrentDesc            *prometheus.Desc
	tempCurrentDesc           *prometheus.Desc
	tempCriticalThresholdDesc *prometheus.Desc
	tempFatalThresholdDesc    *prometheus.Desc
	tempHealthyDesc    *prometheus.Desc
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

func Collect(parentPath string, cl client.Client, ch chan<- prometheus.Metric) error {
	th := Thermal{}

	err := cl.Get(parentPath+"/Thermal", &th)
	if err != nil {
		return errors.Wrap(err, "could not get thermal data")
	}

	for _, f := range th.Fans {
		collectForFan(cl.HostName(), &f, ch)
	}

	for _, t := range th.Temperatures {
		collectForTemperature(cl.HostName(), &t, ch)
	}

	return nil
}

func collectForFan(hostName string, f *Fan, ch chan<- prometheus.Metric) {
	l := []string{hostName, f.Name}
	ch <- prometheus.MustNewConstMetric(fanHealthyDesc, prometheus.GaugeValue, f.Status.HealthValue(), l...)
	ch <- prometheus.MustNewConstMetric(fanEnabledDesc, prometheus.GaugeValue, f.Status.EnabledValue(), l...)
	ch <- prometheus.MustNewConstMetric(fanCurrentDesc, prometheus.GaugeValue, f.CurrentReading, l...)
}

func collectForTemperature(hostName string, t *Temperature, ch chan<- prometheus.Metric) {
	l := []string{hostName, t.Name}
	ch <- prometheus.MustNewConstMetric(tempCurrentDesc, prometheus.GaugeValue, t.ReadingCelsius, l...)
	ch <- prometheus.MustNewConstMetric(tempCriticalThresholdDesc, prometheus.GaugeValue, t.UpperThresholdCritical, l...)
	ch <- prometheus.MustNewConstMetric(tempFatalThresholdDesc, prometheus.GaugeValue, t.UpperThresholdFatal, l...)
	ch <- prometheus.MustNewConstMetric(tempHealthyDesc, prometheus.GaugeValue, t.Status.HealthValue(), l...)
}
