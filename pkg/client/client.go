// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

type Client interface {
	HostName() string
	Get(ressource string, obj interface{}) error
}
