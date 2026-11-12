// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

// Package redfish contains types shared by Redfish resources
package redfish

import (
	"strings"
)

// Link is a reference to a resource
type Link struct {
	ID string `json:"@odata.id"`
}

// Collection is a resource collection
type Collection struct {
	Members []Link `json:"Members"`
}

// Status describes the status and health of a resource
type Status struct {
	Health       string `json:"Health"`
	HealthRollup string `json:"HealthRollup"`
	State        string `json:"State"`
}

// IsAbsent returns true if the resource is not present (e.g. an empty slot or bay)
func (s *Status) IsAbsent() bool {
	return s.State == "Absent"
}

// HealthValue returns 1 if the resource is healthy, 0 otherwise
func (s *Status) HealthValue() float64 {
	return HealthValue(s.Health)
}

// HealthValue returns 1 if health is OK, 0 otherwise
func HealthValue(health string) float64 {
	if strings.ToUpper(health) == "OK" {
		return 1
	}

	return 0
}

// EnabledValue returns 1 if the resource is enabled, 0 otherwise
func (s *Status) EnabledValue() float64 {
	if s.State == "Enabled" {
		return 1
	}

	return 0
}
