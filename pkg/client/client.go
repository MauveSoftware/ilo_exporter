// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2020. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import "context"

type Client interface {
	HostName() string
	Get(ctx context.Context, ressource string, obj interface{}) error
}
