// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
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
	pwr := Power{}
	if err := s.cl.Get(ctx, s.chassisPath+"/Power", &pwr); err != nil {
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
