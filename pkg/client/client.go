// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import "context"

// Client retrieves resources from the Redfish API
type Client interface {
	// HostName returns the name of the host
	HostName() string

	// Get retrieves the resource at path and unmarshals it into obj
	Get(ctx context.Context, path string, obj any) error

	// Requests returns the number of requests sent
	Requests() int64
}
