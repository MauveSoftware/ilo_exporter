// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package thermal

import (
	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
)

type Fan struct {
	Name           string        `json:"FanName"`
	CurrentReading float64       `json:"CurrentReading"`
	Status         common.Status `json:"Status"`
}
