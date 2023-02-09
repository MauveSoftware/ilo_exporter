// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package power

type Power struct {
	PowerConsumedWatts float64 `json:"PowerConsumedWatts"`
	Metrics            struct {
		AverageConsumedWatts float64 `json:"AverageConsumedWatts"`
		MaxConsumedWatts     float64 `json:"MaxConsumedWatts"`
		MinConsumedWatts     float64 `json:"MinConsumedWatts"`
	} `json:"PowerMetrics"`
	PowerSupplies []PowerSupply `json:"PowerSupplies"`
}
