// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

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
	MemberID               string         `json:"MemberId"`
	Name                   string         `json:"Name"`
	ReadingCelsius         float64        `json:"ReadingCelsius"`
	UpperThresholdCritical float64        `json:"UpperThresholdCritical"`
	UpperThresholdFatal    float64        `json:"UpperThresholdFatal"`
	Status                 redfish.Status `json:"Status"`
}

// Fan is a fan of the thermal resource. Legacy fields are used by iLO 4.
type Fan struct {
	MemberID       string         `json:"MemberId"`
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

	fans := slices.DeleteFunc(th.Fans, func(f Fan) bool { return !isOperational(f.Status) })
	fanNames := uniqueNames(fans, (*Fan).Name, func(f *Fan) string { return f.MemberID })
	for i, f := range fans {
		s.emit(fanHealthyDesc, f.Status.HealthValue(), s.host, fanNames[i])
		s.emit(fanEnabledDesc, f.Status.EnabledValue(), s.host, fanNames[i])
		s.emit(fanCurrentDesc, f.Reading(), s.host, fanNames[i])
	}

	temps := slices.DeleteFunc(th.Temperatures, func(t Temperature) bool {
		return t.Status.IsAbsent() || t.Status.State == "Offline"
	})
	tempNames := uniqueNames(temps, func(t *Temperature) string { return t.Name }, func(t *Temperature) string { return t.MemberID })
	for i, t := range temps {
		s.emit(tempCurrentDesc, t.ReadingCelsius, s.host, tempNames[i])
		s.emit(tempCriticalThresholdDesc, t.UpperThresholdCritical, s.host, tempNames[i])
		s.emit(tempFatalThresholdDesc, t.UpperThresholdFatal, s.host, tempNames[i])
		s.emit(tempHealthyDesc, t.Status.HealthValue(), s.host, tempNames[i])
	}

	return nil
}

// isOperational returns false for fans which are not present or offline
func isOperational(st redfish.Status) bool {
	return !st.IsAbsent() && st.State != "UnavailableOffline" && st.State != "Offline"
}

// uniqueNames returns the name label of each sensor. Some firmware (e.g. iLO 5 v2.72) reports different sensors
// with the same name, which would result in identical series of which all but one are dropped. Names occurring
// more than once are suffixed with the ID of the sensor (e.g. "23-HD Controller-I/O controller #24"), all other
// names are kept as they are.
func uniqueNames[T any](items []T, name, id func(*T) string) []string {
	names := make([]string, len(items))
	count := make(map[string]int, len(items))
	for i := range items {
		names[i] = name(&items[i])
		count[names[i]]++
	}

	for i, n := range names {
		if count[n] < 2 {
			continue
		}

		suffix := id(&items[i])
		if suffix == "" {
			suffix = strconv.Itoa(i)
		}
		names[i] = n + " #" + suffix
	}

	return names
}

// ThermalSubsystem is the thermal subsystem of a chassis (Redfish 2020.4 and later)
type ThermalSubsystem struct {
	Fans redfish.Link `json:"Fans"`
}

// SubsystemFan is a fan of the thermal subsystem
type SubsystemFan struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	SpeedPercent struct {
		Reading float64 `json:"Reading"`
	} `json:"SpeedPercent"`
	Status redfish.Status `json:"Status"`
}

// Sensor is a sensor of a chassis (Redfish 2018.3 and later)
type Sensor struct {
	ID           string  `json:"Id"`
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
		sensors = slices.DeleteFunc(sensors, func(t Sensor) bool {
			return !t.isTemperature() || t.Status.IsAbsent() || t.Status.State == "Offline"
		})
		names := uniqueNames(sensors, func(t *Sensor) string { return t.Name }, func(t *Sensor) string { return t.ID })
		for i, t := range sensors {
			s.emit(tempCurrentDesc, t.Reading, s.host, names[i])
			s.emit(tempCriticalThresholdDesc, t.Thresholds.UpperCritical.Reading, s.host, names[i])
			s.emit(tempFatalThresholdDesc, t.Thresholds.UpperFatal.Reading, s.host, names[i])
			s.emit(tempHealthyDesc, t.Status.HealthValue(), s.host, names[i])
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
	fans = slices.DeleteFunc(fans, func(f SubsystemFan) bool { return !isOperational(f.Status) })
	names := uniqueNames(fans, func(f *SubsystemFan) string { return f.Name }, func(f *SubsystemFan) string { return f.ID })
	for i, f := range fans {
		s.emit(fanHealthyDesc, f.Status.HealthValue(), s.host, names[i])
		s.emit(fanEnabledDesc, f.Status.EnabledValue(), s.host, names[i])
		s.emit(fanCurrentDesc, f.SpeedPercent.Reading, s.host, names[i])
	}

	if err != nil {
		return fmt.Errorf("could not get fans: %w", err)
	}

	return nil
}
