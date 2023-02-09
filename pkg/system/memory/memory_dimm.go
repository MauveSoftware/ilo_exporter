// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory
type MemoryDIMM struct {
	Name       string `json:"Name"`
	DIMMStatus string `json:"DIMMStatus"`
	SizeMB     uint64 `json:"SizeMB"`
}
