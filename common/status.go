package common

import (
	"strings"
)

type Status struct {
	Health string `json:"Health"`
	State  string `json:"State"`
}

func (s *Status) HealthValue() float64 {
	if strings.ToUpper(s.Health) == "OK" {
		return 1
	}

	return 0
}

func (s *Status) EnabledValue() float64 {
	if s.State == "Enabled" {
		return 1
	}

	return 0
}
