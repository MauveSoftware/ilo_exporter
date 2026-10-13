// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"fmt"

	"github.com/MauveSoftware/ilo_exporter/pkg/redfish"
)

const memoryPrefix = prefix + "memory_"

var (
	memoryHealthyDesc = newDesc(memoryPrefix+"healthy", "Health status of the memory", "host")        //nolint:unused // described, but not collected yet
	memoryTotalDesc   = newDesc(memoryPrefix+"total_byte", "Total memory installed in bytes", "host") //nolint:unused // described, but not collected yet
	dimmHealthyDesc   = newDesc(memoryPrefix+"dimm_healthy", "Health status of processor", "host", "name")
	dimmSizeDesc      = newDesc(memoryPrefix+"dimm_byte", "DIMM size in bytes", "host", "name")
)

// MemoryDIMM is a memory resource. Legacy fields are used by iLO 4.
type MemoryDIMM struct {
	Name          string         `json:"Name"`
	Oem           MemoryOem      `json:"Oem"`
	StatusCurrent redfish.Status `json:"Status"`
	StatusLegacy  string         `json:"DIMMStatus"`
	SizeMBCurrent uint64         `json:"CapacityMiB"`
	SizeMBLegacy  uint64         `json:"SizeMB"`
}

// MemoryOem contains the OEM specific properties of a memory resource
type MemoryOem struct {
	HPE HPEMemStatus `json:"Hpe"`
}

// HPEMemStatus contains the HPE specific status of a memory resource
type HPEMemStatus struct {
	DIMMStatus string `json:"DIMMStatus"` // e.g. "GoodInUse"
}

// HealthValue returns 1 if the DIMM is healthy, 0 otherwise
func (s *HPEMemStatus) HealthValue() float64 {
	if s.DIMMStatus == "GoodInUse" {
		return 1
	}

	return 0
}

func (m *MemoryDIMM) isLegacy() bool {
	return len(m.StatusLegacy) > 0
}

func (m *MemoryDIMM) legacyHealthValue() float64 {
	if m.StatusLegacy == "GoodInUse" {
		return 1
	}

	return 0
}

// IsValid returns false for DIMMs which should not be reported
func (m *MemoryDIMM) IsValid() bool {
	if m.isLegacy() {
		return m.StatusLegacy != "Unknown"
	}

	return len(m.StatusCurrent.State) > 0 || len(m.Oem.HPE.DIMMStatus) > 0
}

// HealthValue returns 1 if the DIMM is healthy, 0 otherwise
func (m *MemoryDIMM) HealthValue() float64 {
	if m.isLegacy() {
		return m.legacyHealthValue()
	}

	val := m.StatusCurrent.HealthValue()
	if val > 0 {
		return val
	}

	return m.Oem.HPE.HealthValue()
}

// SizeMB returns the size in MiB
func (m *MemoryDIMM) SizeMB() uint64 {
	if m.isLegacy() {
		return m.SizeMBLegacy
	}

	return m.SizeMBCurrent
}

func collectMemory(ctx context.Context, s *scrape) error {
	path := s.systemLink(func(sys *System) string { return sys.Memory.ID }, "Memory")
	if path == "" {
		return nil
	}

	dimms, err := getMembers[MemoryDIMM](ctx, s, path)
	for _, d := range dimms {
		if !d.IsValid() {
			continue
		}

		s.emit(dimmHealthyDesc, d.HealthValue(), s.host, d.Name)
		s.emit(dimmSizeDesc, float64(d.SizeMB()<<20), s.host, d.Name)
	}

	if err != nil {
		return fmt.Errorf("could not get memory information: %w", err)
	}

	return nil
}
