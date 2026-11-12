// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"fmt"

	"github.com/MauveSoftware/ilo_exporter/pkg/redfish"
)

var (
	powerUpDesc    = newDesc(prefix+"power_up", "Power status", "host")
	systemInfoDesc = newDesc(prefix+"system_info", "System Info", "host", "uuid", "serial", "sku", "model", "host_name", "bios_version")
)

// System is a computer system resource
type System struct {
	PowerState   string `json:"PowerState"`
	UUID         string `json:"UUID"`
	SerialNumber string `json:"SerialNumber"`
	SKU          string `json:"SKU"`
	Model        string `json:"Model"`
	HostName     string `json:"HostName"`
	BiosVersion  string `json:"BiosVersion"`

	MemorySummary struct {
		TotalSystemMemoryGiB *float64       `json:"TotalSystemMemoryGiB"`
		Status               redfish.Status `json:"Status"`
	} `json:"MemorySummary"`

	Memory     redfish.Link `json:"Memory"`
	Processors redfish.Link `json:"Processors"`
	Storage    redfish.Link `json:"Storage"`

	Links struct {
		Chassis   []redfish.Link `json:"Chassis"`
		ManagedBy []redfish.Link `json:"ManagedBy"`
	} `json:"Links"`

	Oem struct {
		Hpe systemOem `json:"Hpe"`
		Hp  systemOem `json:"Hp"` // iLO 4
	} `json:"Oem"`
}

type systemOem struct {
	Links struct {
		SmartStorage redfish.Link `json:"SmartStorage"`
	} `json:"Links"`
}

// PowerUpValue returns 1 if the system is powered on, 0 otherwise
func (s *System) PowerUpValue() float64 {
	if s.PowerState == "On" {
		return 1
	}

	return 0
}

// SmartStorageLink returns the link to the HPE SmartStorage API (iLO 4 and 5 only)
func (s *System) SmartStorageLink() string {
	if l := s.Oem.Hpe.Links.SmartStorage.ID; l != "" {
		return l
	}

	return s.Oem.Hp.Links.SmartStorage.ID
}

func collectSystem(ctx context.Context, s *scrape) error {
	sys := System{}
	if err := s.cl.Get(ctx, s.systemPath, &sys); err != nil {
		s.checkStale(err)
		return fmt.Errorf("could not get system: %w", err)
	}

	s.emit(powerUpDesc, sys.PowerUpValue(), s.host)
	s.emit(systemInfoDesc, 1, s.host, sys.UUID, sys.SerialNumber, sys.SKU, sys.Model, sys.HostName, sys.BiosVersion)

	if v := sys.MemorySummary.TotalSystemMemoryGiB; v != nil {
		s.emit(memoryTotalDesc, *v*(1<<30), s.host)
	}
	if st := sys.MemorySummary.Status; st.HealthRollup != "" {
		s.emit(memoryHealthyDesc, redfish.HealthValue(st.HealthRollup), s.host)
	}

	s.system = &sys
	if s.topology == nil && len(sys.Links.Chassis) > 0 {
		s.chassisPath = sys.Links.Chassis[0].ID
	}

	return nil
}

// systemLink returns the link of the system resource or the default path relative to the system if the system
// resource is unknown. If the system is known but does not contain the link, an empty string is returned.
func (s *scrape) systemLink(link func(*System) string, defaultPath string) string {
	if s.system == nil {
		return s.systemPath + "/" + defaultPath
	}

	return link(s.system)
}
