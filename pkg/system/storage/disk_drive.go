// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import (
	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
)

type DiskDrive struct {
	InterfaceType             string        `json:"InterfaceType"`
	Model                     string        `json:"Model"`
	Location                  string        `json:"Location"`
	CurrentTemperatureCelsius float64       `json:"CurrentTemperatureCelsius"`
	CapacityGB                uint64        `json:"CapacityGB"`
	RotationalSpeedRpm        float64       `json:"RotationalSpeedRpm"`
	Status                    common.Status `json:"Status"`
}
