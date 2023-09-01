// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory

import "github.com/MauveSoftware/ilo_exporter/pkg/common"

type MemoryDIMM struct {
	Name          string        `json:"Name"`
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

	return len(m.StatusCurrent.State) > 0
}

func (m *MemoryDIMM) HealthValue() float64 {
	if m.isLegacy() {
		return m.legacyHealthValue()
	}

	return m.StatusCurrent.HealthValue()
}

func (m *MemoryDIMM) SizeMB() uint64 {
	if m.isLegacy() {
		return m.SizeMBLegacy
	}

	return m.SizeMBCurrent
}
