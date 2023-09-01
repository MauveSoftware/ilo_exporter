// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package thermal

import (
	"github.com/MauveSoftware/ilo_exporter/pkg/common"
)

type Fan struct {
	NameCurrent    string        `json:"Name"`
	NameLegacy     string        `json:"FanName"`
	ReadingCurrent float64       `json:"Reading"`
	ReadingLegacy  float64       `json:"CurrentReading"`
	Status         common.Status `json:"Status"`
}

func (f *Fan) isLegacy() bool {
	return len(f.NameLegacy) > 0
}

func (f *Fan) Name() string {
	if f.isLegacy() {
		return f.NameLegacy
	}

	return f.NameCurrent
}

func (f *Fan) Reading() float64 {
	if f.isLegacy() {
		return f.ReadingLegacy
	}

	return f.ReadingCurrent
}
