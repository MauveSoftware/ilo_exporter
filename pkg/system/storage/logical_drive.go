// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import "github.com/MauveSoftware/ilo4_exporter/pkg/common"

type LogicalDrive struct {
	LogicalDriveName string        `json:"LogicalDriveName"`
	Raid             string        `json:"Raid"`
	CapacityMiB      uint64        `json:"CapacityMiB"`
	Status           common.Status `json:"Status"`
}
