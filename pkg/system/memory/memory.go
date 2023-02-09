// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory

type Memory struct {
	MemorySummary struct {
		Status struct {
			HealthRollUp string `json:"HealthRollUp"`
		} `json:"Status"`
		TotalSystemMemoryGiB uint64 `json:"TotalSystemMemoryGiB"`
	} `json:"MemorySummary"`
}
