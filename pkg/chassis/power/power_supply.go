// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package power

import (
	"github.com/MauveSoftware/ilo5_exporter/pkg/common"
)

type PowerSupply struct {
	SerialNumber string        `json:"SerialNumber"`
	Status       common.Status `json:"Status"`
}
