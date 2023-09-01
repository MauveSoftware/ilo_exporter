// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import (
	"github.com/MauveSoftware/ilo_exporter/pkg/common"
)

type DiskDrive struct {
	MediaType  string        `json:"MediaType"`
	Model      string        `json:"Model"`
	Location   Location      `json:"Location"`
	CapacityB  uint64        `json:"CapacityBytes"`
	CapacityMB uint64        `json:"CapacityMiB"`
	Status     common.Status `json:"Status"`
}

func (d *DiskDrive) CapacityBytes() float64 {
	if d.CapacityMB > 0 {
		return float64(d.CapacityMB << 10)
	}

	return float64(d.CapacityB)
}
