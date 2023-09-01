// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
)

type DummyClient struct {
	values map[string]string
}

// NewDummy returns a new dummy client
func NewDummy() *DummyClient {
	return &DummyClient{
		values: make(map[string]string),
	}
}

func (cl *DummyClient) HostName() string {
	return "Dummy"
}

// SetResponse sets an dummy response for an ressource path
func (cl *DummyClient) SetResponse(ressource string, value string) {
	cl.values[ressource] = value
}

// Get parses the dummy string for an given ressource and unmarshals the json
func (cl *DummyClient) Get(context context.Context, ressource string, obj interface{}) error {
	return json.Unmarshal([]byte(cl.values[ressource]), obj)
}
