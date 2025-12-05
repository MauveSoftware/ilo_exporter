// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory

import "github.com/MauveSoftware/ilo_exporter/pkg/common"

type Oem struct {
	HPE HPEMemStatus `json:"Hpe"`
}

type HPEMemStatus struct {
	Attributes             []string `json:"Attributes"`
	BaseModuleType         string   `json:"BaseModuleType"`
	DIMMManufacturingDate  string   `json:"DIMMManufacturingDate"`
	DIMMStatus             string   `json:"DIMMStatus"` // "GoodInUse"
	MaxOperatingSpeedMTs   uint     `json:"MaxOperatingSpeedMTs"`
	MinimumVoltageVoltsX10 uint     `json:"MinimumVoltageVoltsX10"`
	PartNumber             string   `json:"PartNumber"`
	VendorName             string   `json:"VendorName"`
}

func (s *HPEMemStatus) HealthValue() float64 {
	if s.DIMMStatus == "GoodInUse" {
		return 1
	}

	return 0
}

type MemoryStatus struct{}

type MemoryDIMM struct {
	Name          string        `json:"Name"`
	Oem           Oem           `json:"Oem"`
	StatusCurrent common.Status `json:"Status"`
	StatusLegacy  string        `json:"DIMMStatus"`
	SizeMBCurrent uint64        `json:"CapacityMiB"`
	SizeMBLegacy  uint64        `json:"SizeMB"`
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

func (m *MemoryDIMM) IsValid() bool {
	if m.isLegacy() {
		return m.StatusLegacy != "Unknown"
	}

	return len(m.StatusCurrent.State) > 0 || len(m.Oem.HPE.DIMMStatus) > 0
}

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

func (m *MemoryDIMM) SizeMB() uint64 {
	if m.isLegacy() {
		return m.SizeMBLegacy
	}

	return m.SizeMBCurrent
}
