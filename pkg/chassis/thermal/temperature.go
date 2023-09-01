// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package thermal

import (
	"github.com/MauveSoftware/ilo5_exporter/pkg/common"
)

type Temperature struct {
	Name                   string        `json:"Name"`
	ReadingCelsius         float64       `json:"ReadingCelsius"`
	UpperThresholdCritical float64       `json:"UpperThresholdCritical"`
	UpperThresholdFatal    float64       `json:"UpperThresholdFatal"`
	Status                 common.Status `json:"Status"`
}
