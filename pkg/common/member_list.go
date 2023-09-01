// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package common

type MemberList struct {
	Members []Member
}

type Member struct {
	Path string `json:"@odata.id"`
}
