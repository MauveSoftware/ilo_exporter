// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import "github.com/MauveSoftware/ilo5_exporter/pkg/common"

type StorageInfo struct {
	Drives []common.Member `json:"Drives"`
}
