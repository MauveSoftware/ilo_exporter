// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package power

type Power struct {
	PowerControl []struct {
		ID                 string  `json:"MemberId"`
		PowerCapacityWatts float64 `json:"PowerCapacityWatts"`
		PowerConsumedWatts float64 `json:"PowerConsumedWatts"`
		Metrics            struct {
			AverageConsumedWatts float64 `json:"AverageConsumedWatts"`
			MaxConsumedWatts     float64 `json:"MaxConsumedWatts"`
			MinConsumedWatts     float64 `json:"MinConsumedWatts"`
		} `json:"PowerMetrics"`
	} `json:"PowerControl"`
	PowerSupplies []PowerSupply `json:"PowerSupplies"`
}
