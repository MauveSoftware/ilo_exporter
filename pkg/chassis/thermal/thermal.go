// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package thermal

type Thermal struct {
	Temperatures []Temperature `json:"Temperatures"`
	Fans         []Fan         `json:"Fans"`
}
