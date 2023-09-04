// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package system

type System struct {
	PowerState   string `json:"PowerState"`
	UUID         string `json:"UUID"`
	SerialNumber string `json:"SerialNumber"`
	SKU          string `json:"SKU"`
	Model        string `json:"Model"`
	HostName     string `json:"HostName"`
  BiosVersion string `json:"BiosVersion"`
}

func (s *System) PowerUpValue() float64 {
	if s.PowerState == "On" {
		return 1
	}

	return 0
}
