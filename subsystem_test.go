// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
)

// readingLine matches readings of sensors, which differ between the resources of a mockup since they were
// not captured at the same time
var readingLine = regexp.MustCompile(`^(ilo_chassis_(?:temperature_current|fan_current_percent)\{[^}]*\}) .*$`)

// chassisLines returns the fan, temperature and power metrics of a scrape
func chassisLines(body, host string) []string {
	var res []string
	for _, l := range strings.Split(strings.ReplaceAll(body, host, "ilo.test"), "\n") {
		if strings.HasPrefix(l, "ilo_chassis_fan") || strings.HasPrefix(l, "ilo_chassis_temperature") ||
			(strings.HasPrefix(l, "ilo_power_") && !strings.HasPrefix(l, "ilo_power_up")) {
			res = append(res, readingLine.ReplaceAllString(l, "$1 <reading>"))
		}
	}

	return res
}

// TestSubsystemFallback compares metrics retrieved from the resources replacing the deprecated power and thermal
// resources with the metrics retrieved from the deprecated resources
func TestSubsystemFallback(t *testing.T) {
	tests := map[string]struct {
		// power metrics available in the fixture without the power resource
		powerMetrics []string
	}{
		// PowerSubsystem, but no EnvironmentMetrics
		"ilo6": {powerMetrics: []string{"ilo_power_capacity_watt", "ilo_power_supply_enabled", "ilo_power_supply_healthy"}},
		// neither PowerSubsystem nor EnvironmentMetrics (added in iLO 7 v1.18)
		"ilo7": {},
	}

	for gen, tt := range tests {
		t.Run(gen, func(t *testing.T) {
			legacySrv := fakeilo.New(t, mockupDir(gen))
			legacy := chassisLines(scrape(t, newTestExporter(t), legacySrv.Host()), legacySrv.Host())

			srv := fakeilo.New(t, mockupDir(gen),
				fakeilo.WithoutProperty("/redfish/v1/Chassis/1", "Power"),
				fakeilo.WithoutProperty("/redfish/v1/Chassis/1", "Thermal"))
			got := chassisLines(scrape(t, newTestExporter(t), srv.Host()), srv.Host())

			var want []string
			for _, l := range legacy {
				name := l[:strings.IndexAny(l, "{ ")]
				if !strings.HasPrefix(name, "ilo_power_") || slices.Contains(tt.powerMetrics, name) {
					want = append(want, l)
				}
			}

			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("metrics differ:\n%s", diff(strings.Join(want, "\n"), strings.Join(got, "\n")))
			}

			for _, r := range srv.Requests() {
				if strings.HasSuffix(r, "/Chassis/1/Power") || strings.HasSuffix(r, "/Chassis/1/Thermal") {
					t.Errorf("unexpected request of deprecated resource: %s", r)
				}
			}
		})
	}
}

func TestEnvironmentMetrics(t *testing.T) {
	dir := writeMockup(t, map[string]string{
		"":                             `{"Systems": {"@odata.id": "/redfish/v1/Systems"}}`,
		"Systems":                      `{"Members": [{"@odata.id": "/redfish/v1/Systems/1"}]}`,
		"Systems/1":                    `{"PowerState": "On", "Links": {"Chassis": [{"@odata.id": "/redfish/v1/Chassis/1"}]}}`,
		"Chassis/1":                    `{"EnvironmentMetrics": {"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics"}}`,
		"Chassis/1/EnvironmentMetrics": `{"PowerWatts": {"Reading": 317.5}}`,
	})
	srv := fakeilo.New(t, dir)

	body := scrape(t, newTestExporter(t), srv.Host())

	if want := `ilo_power_current_watt{host="` + srv.Host() + `",id="0"} 317.5`; !strings.Contains(body, want) {
		t.Errorf("expected %s, got:\n%s", want, body)
	}
}
