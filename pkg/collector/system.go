// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"fmt"
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
}

// PowerUpValue returns 1 if the system is powered on, 0 otherwise
func (s *System) PowerUpValue() float64 {
	if s.PowerState == "On" {
		return 1
	}

	return 0
}

func collectSystem(ctx context.Context, s *scrape) error {
	sys := System{}
	err := s.cl.Get(ctx, s.systemPath, &sys)

	s.emit(powerUpDesc, sys.PowerUpValue(), s.host)
	s.emit(systemInfoDesc, 1, s.host, sys.UUID, sys.SerialNumber, sys.SKU, sys.Model, sys.HostName, sys.BiosVersion)

	if err != nil {
		return fmt.Errorf("could not get system: %w", err)
	}

	return nil
}
