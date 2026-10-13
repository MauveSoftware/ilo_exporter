// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"errors"
	"fmt"

	"github.com/MauveSoftware/ilo_exporter/pkg/redfish"
)

const chassisPrefix = prefix + "chassis_"

var (
	fanHealthyDesc            = newDesc(chassisPrefix+"fan_healthy", "Health status of the fan", "host", "name")
	fanEnabledDesc            = newDesc(chassisPrefix+"fan_enabled", "Status of the fan", "host", "name")
	fanCurrentDesc            = newDesc(chassisPrefix+"fan_current_percent", "Current power in percent", "host", "name")
	tempCurrentDesc           = newDesc(chassisPrefix+"temperature_current", "Current temperature in degree celsius", "host", "name")
	tempCriticalThresholdDesc = newDesc(chassisPrefix+"temperature_critical", "Critcal temperature threshold in degree celsius", "host", "name")
	tempFatalThresholdDesc    = newDesc(chassisPrefix+"temperature_fatal", "Fatal temperature threshold in degree celsius", "host", "name")
	tempHealthyDesc           = newDesc(chassisPrefix+"temperature_healthy", "Health status of the temperature sensor", "host", "name")
)

// Thermal is the (deprecated, but most efficient) thermal resource of a chassis
type Thermal struct {
	Temperatures []Temperature `json:"Temperatures"`
	Fans         []Fan         `json:"Fans"`
}

// Temperature is a temperature sensor of the thermal resource
type Temperature struct {
	Name                   string         `json:"Name"`
	ReadingCelsius         float64        `json:"ReadingCelsius"`
	UpperThresholdCritical float64        `json:"UpperThresholdCritical"`
	UpperThresholdFatal    float64        `json:"UpperThresholdFatal"`
	Status                 redfish.Status `json:"Status"`
}

// Fan is a fan of the thermal resource. Legacy fields are used by iLO 4.
type Fan struct {
	NameCurrent    string         `json:"Name"`
	NameLegacy     string         `json:"FanName"`
	ReadingCurrent float64        `json:"Reading"`
	ReadingLegacy  float64        `json:"CurrentReading"`
	Status         redfish.Status `json:"Status"`
}

func (f *Fan) isLegacy() bool {
	return len(f.NameLegacy) > 0
}

// Name returns the name of the fan
func (f *Fan) Name() string {
	if f.isLegacy() {
		return f.NameLegacy
	}

	return f.NameCurrent
}

// Reading returns the fan speed in percent
func (f *Fan) Reading() float64 {
	if f.isLegacy() {
		return f.ReadingLegacy
	}

	return f.ReadingCurrent
}

func collectThermal(ctx context.Context, s *scrape) error {
	path := s.chassisPath + "/Thermal"
	if s.topology != nil {
		path = s.topology.Chassis.Thermal
	}
	if path == "" {
		return collectThermalSubsystem(ctx, s)
	}

	th := Thermal{}
	if err := s.cl.Get(ctx, path, &th); err != nil {
		s.checkStale(err)
		return fmt.Errorf("could not get thermal data: %w", err)
	}

	for _, f := range th.Fans {
		if f.Status.IsAbsent() || f.Status.State == "UnavailableOffline" || f.Status.State == "Offline" {
			continue
		}

		s.emit(fanHealthyDesc, f.Status.HealthValue(), s.host, f.Name())
		s.emit(fanEnabledDesc, f.Status.EnabledValue(), s.host, f.Name())
		s.emit(fanCurrentDesc, f.Reading(), s.host, f.Name())
	}

	for _, t := range th.Temperatures {
		if t.Status.State == "Absent" || t.Status.State == "Offline" {
			continue
		}

		s.emit(tempCurrentDesc, t.ReadingCelsius, s.host, t.Name)
		s.emit(tempCriticalThresholdDesc, t.UpperThresholdCritical, s.host, t.Name)
		s.emit(tempFatalThresholdDesc, t.UpperThresholdFatal, s.host, t.Name)
		s.emit(tempHealthyDesc, t.Status.HealthValue(), s.host, t.Name)
	}

	return nil
}

// ThermalSubsystem is the thermal subsystem of a chassis (Redfish 2020.4 and later)
type ThermalSubsystem struct {
	Fans redfish.Link `json:"Fans"`
}

// SubsystemFan is a fan of the thermal subsystem
type SubsystemFan struct {
	Name         string `json:"Name"`
	SpeedPercent struct {
		Reading float64 `json:"Reading"`
	} `json:"SpeedPercent"`
	Status redfish.Status `json:"Status"`
}

// Sensor is a sensor of a chassis (Redfish 2018.3 and later)
type Sensor struct {
	Name         string  `json:"Name"`
	Reading      float64 `json:"Reading"`
	ReadingType  string  `json:"ReadingType"`
	ReadingUnits string  `json:"ReadingUnits"`
	Thresholds   struct {
		UpperCritical struct {
			Reading float64 `json:"Reading"`
		} `json:"UpperCritical"`
		UpperFatal struct {
			Reading float64 `json:"Reading"`
		} `json:"UpperFatal"`
	} `json:"Thresholds"`
	Status redfish.Status `json:"Status"`
}

// isTemperature returns true for temperature sensors. iLO 6 and 7 only provide temperature sensors
// and do not set the reading type.
func (s *Sensor) isTemperature() bool {
	if s.ReadingType != "" {
		return s.ReadingType == "Temperature"
	}

	return s.ReadingUnits == "" || s.ReadingUnits == "Cel"
}

// collectThermalSubsystem collects fan and temperature metrics using the resources replacing the deprecated
// thermal resource. The thermal resource is preferred, since it contains all metrics in a single resource.
// Values not provided are reported as 0 like for the thermal resource, so the metrics do not depend on the
// resources they were retrieved from.
func collectThermalSubsystem(ctx context.Context, s *scrape) error {
	links := s.topology.Chassis

	var errs []error
	if links.ThermalSubsystem != "" {
		errs = append(errs, collectSubsystemFans(ctx, s, links.ThermalSubsystem))
	}

	if links.Sensors != "" {
		sensors, err := getMembers[Sensor](ctx, s, links.Sensors)
		for _, t := range sensors {
			if !t.isTemperature() || t.Status.IsAbsent() || t.Status.State == "Offline" {
				continue
			}

			s.emit(tempCurrentDesc, t.Reading, s.host, t.Name)
			s.emit(tempCriticalThresholdDesc, t.Thresholds.UpperCritical.Reading, s.host, t.Name)
			s.emit(tempFatalThresholdDesc, t.Thresholds.UpperFatal.Reading, s.host, t.Name)
			s.emit(tempHealthyDesc, t.Status.HealthValue(), s.host, t.Name)
		}

		if err != nil {
			errs = append(errs, fmt.Errorf("could not get sensors: %w", err))
		}
	}

	return errors.Join(errs...)
}

func collectSubsystemFans(ctx context.Context, s *scrape, path string) error {
	ts := ThermalSubsystem{}
	if err := s.cl.Get(ctx, path, &ts); err != nil {
		s.checkStale(err)
		return fmt.Errorf("could not get thermal subsystem: %w", err)
	}

	if ts.Fans.ID == "" {
		return nil
	}

	fans, err := getMembers[SubsystemFan](ctx, s, ts.Fans.ID)
	for _, f := range fans {
		if f.Status.IsAbsent() || f.Status.State == "UnavailableOffline" || f.Status.State == "Offline" {
			continue
		}

		s.emit(fanHealthyDesc, f.Status.HealthValue(), s.host, f.Name)
		s.emit(fanEnabledDesc, f.Status.EnabledValue(), s.host, f.Name)
		s.emit(fanCurrentDesc, f.SpeedPercent.Reading, s.host, f.Name)
	}

	if err != nil {
		return fmt.Errorf("could not get fans: %w", err)
	}

	return nil
}
