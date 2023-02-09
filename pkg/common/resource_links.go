// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package common

type ResourceLinks struct {
	Links struct {
		Members []struct {
			Href string `json:"href"`
		} `json:"Member"`
	} `json:"links"`
}