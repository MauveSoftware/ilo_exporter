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

const powerPrefix = prefix + "power_"

var (
	powerCurrentDesc       = newDesc(powerPrefix+"current_watt", "Current power consumption in watt", "host", "id")
	powerAvgDesc           = newDesc(powerPrefix+"average_watt", "Average power consumption in watt", "host", "id")
	powerMinDesc           = newDesc(powerPrefix+"min_watt", "Minimum power consumption in watt", "host", "id")
	powerMaxDesc           = newDesc(powerPrefix+"max_watt", "Maximum power consumption in watt", "host", "id")
	powerCapacityDesc      = newDesc(powerPrefix+"capacity_watt", "Power capacity in watt", "host", "id")
	powerSupplyHealthyDesc = newDesc(powerPrefix+"supply_healthy", "Health status of the power supply", "host", "serial")
	powerSupplyEnabledDesc = newDesc(powerPrefix+"supply_enabled", "Status of the power supply", "host", "serial")
)

// Power is the (deprecated, but most efficient) power resource of a chassis
type Power struct {
	PowerControl []struct {
		ID                 string  `json:"MemberId"`
		PowerCapacityWatts float64 `json:"PowerCapacityWatts"`
		PowerConsumedWatts float64 `json:"PowerConsumedWatts"`
		Metrics            struct {
			AverageConsumedWatts float64 `json:"AverageConsumedWatts"`
			MaxConsumedWatts     float64 `json:"MaxConsumedWatts"`
			MinConsumedWatts     float64 `json:"MinConsumedWatts"`
		} `json:"PowerMetrics"`
	} `json:"PowerControl"`
	PowerSupplies []PowerSupply `json:"PowerSupplies"`
}

// PowerSupply is a power supply of the power resource
type PowerSupply struct {
	SerialNumber string         `json:"SerialNumber"`
	Status       redfish.Status `json:"Status"`
}

func collectPower(ctx context.Context, s *scrape) error {
	path := s.chassisPath + "/Power"
	if s.topology != nil {
		path = s.topology.Chassis.Power
	}
	if path == "" {
		return collectPowerSubsystem(ctx, s)
	}

	pwr := Power{}
	if err := s.cl.Get(ctx, path, &pwr); err != nil {
		s.checkStale(err)
		return fmt.Errorf("could not get power data: %w", err)
	}

	for _, pwc := range pwr.PowerControl {
		s.emit(powerCurrentDesc, pwc.PowerConsumedWatts, s.host, pwc.ID)
		s.emit(powerAvgDesc, pwc.Metrics.AverageConsumedWatts, s.host, pwc.ID)
		s.emit(powerMinDesc, pwc.Metrics.MinConsumedWatts, s.host, pwc.ID)
		s.emit(powerMaxDesc, pwc.Metrics.MaxConsumedWatts, s.host, pwc.ID)
		s.emit(powerCapacityDesc, pwc.PowerCapacityWatts, s.host, pwc.ID)
	}

	for _, sup := range pwr.PowerSupplies {
		if sup.Status.State == "Absent" {
			continue
		}

		s.emit(powerSupplyEnabledDesc, sup.Status.EnabledValue(), s.host, sup.SerialNumber)
		s.emit(powerSupplyHealthyDesc, sup.Status.HealthValue(), s.host, sup.SerialNumber)
	}

	return nil
}

// PowerSubsystem is the power subsystem of a chassis (Redfish 2020.4 and later)
type PowerSubsystem struct {
	CapacityWatts *float64     `json:"CapacityWatts"`
	PowerSupplies redfish.Link `json:"PowerSupplies"`
}

// EnvironmentMetrics are the environment metrics of a chassis (Redfish 2020.4 and later)
type EnvironmentMetrics struct {
	PowerWatts struct {
		Reading *float64 `json:"Reading"`
	} `json:"PowerWatts"`
}

// collectPowerSubsystem collects power metrics using the resources replacing the deprecated power resource.
// The power resource is preferred, since it contains all metrics (incl. average, min and max consumption)
// in a single resource.
func collectPowerSubsystem(ctx context.Context, s *scrape) error {
	// metrics of the chassis have the ID of the first power control of the power resource for continuity
	const id = "0"
	links := s.topology.Chassis

	var errs []error
	if links.EnvironmentMetrics != "" {
		em := EnvironmentMetrics{}
		if err := s.cl.Get(ctx, links.EnvironmentMetrics, &em); err != nil {
			s.checkStale(err)
			errs = append(errs, fmt.Errorf("could not get environment metrics: %w", err))
		} else if em.PowerWatts.Reading != nil {
			s.emit(powerCurrentDesc, *em.PowerWatts.Reading, s.host, id)
		}
	}

	if links.PowerSubsystem != "" {
		errs = append(errs, collectPowerSupplies(ctx, s, links.PowerSubsystem, id))
	}

	return errors.Join(errs...)
}

func collectPowerSupplies(ctx context.Context, s *scrape, path, id string) error {
	ps := PowerSubsystem{}
	if err := s.cl.Get(ctx, path, &ps); err != nil {
		s.checkStale(err)
		return fmt.Errorf("could not get power subsystem: %w", err)
	}

	if ps.CapacityWatts != nil {
		s.emit(powerCapacityDesc, *ps.CapacityWatts, s.host, id)
	}

	if ps.PowerSupplies.ID == "" {
		return nil
	}

	supplies, err := getMembers[PowerSupply](ctx, s, ps.PowerSupplies.ID)
	for _, sup := range supplies {
		if sup.Status.IsAbsent() {
			continue
		}

		s.emit(powerSupplyEnabledDesc, sup.Status.EnabledValue(), s.host, sup.SerialNumber)
		s.emit(powerSupplyHealthyDesc, sup.Status.HealthValue(), s.host, sup.SerialNumber)
	}

	if err != nil {
		return fmt.Errorf("could not get power supplies: %w", err)
	}

	return nil
}
