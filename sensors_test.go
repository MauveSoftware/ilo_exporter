// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
	"github.com/prometheus/common/promslog"
)

// chassisMockup returns a mockup with a chassis linking the given resources (relative to the chassis)
func chassisMockup(links string, resources map[string]string) map[string]string {
	m := map[string]string{
		"":          `{"Systems": {"@odata.id": "/redfish/v1/Systems"}, "ProtocolFeaturesSupported": {"ExpandQuery": {"NoLinks": true}}}`,
		"Systems":   `{"Members": [{"@odata.id": "/redfish/v1/Systems/1"}]}`,
		"Systems/1": `{"PowerState": "On", "Links": {"Chassis": [{"@odata.id": "/redfish/v1/Chassis/1"}]}}`,
		"Chassis/1": `{` + links + `}`,
	}
	for k, v := range resources {
		m[k] = v
	}

	return m
}

func temperature(id, name string, reading int) string {
	return fmt.Sprintf(`{"MemberId": %q, "Name": %q, "ReadingCelsius": %d, "UpperThresholdCritical": 80, "Status": {"Health": "OK", "State": "Enabled"}}`, id, name, reading)
}

func sensor(id, name string, reading int) string {
	return fmt.Sprintf(`{"@odata.type": "#Sensor.v1_7_0.Sensor", "Id": %q, "Name": %q, "Reading": %d, "Thresholds": {"UpperCritical": {"Reading": 80}}, "Status": {"Health": "OK", "State": "Enabled"}}`, id, name, reading)
}

var tempLine = regexp.MustCompile(`(?m)^ilo_chassis_temperature_current\{host="[^"]*",name="([^"]*)"\} (\S+)$`)

func temperatures(body string) []string {
	var res []string
	for _, m := range tempLine.FindAllStringSubmatch(body, -1) {
		res = append(res, m[1]+"="+m[2])
	}
	slices.Sort(res)
	return res
}

// TestDuplicateSensorNames covers firmware reporting different sensors with the same name (seen on iLO 5 v2.72)
func TestDuplicateSensorNames(t *testing.T) {
	want := []string{
		"23-HD Controller #22=76",
		"23-HD Controller-I/O controller #40=69",
		"23-HD Controller-I/O controller #41=54",
		"23-HD Controller-I/O controller #42=56",
		"29-PCI 1 #28=73",
		"29-PCI 1-I/O module #43=71",
		"29-PCI 1-I/O module #44=54",
	}
	// names occurring once must not change
	want[0] = "23-HD Controller=76"
	want[4] = "29-PCI 1=73"
	slices.Sort(want)

	thermal := `{"Temperatures": [` + strings.Join([]string{
		temperature("22", "23-HD Controller", 76),
		temperature("28", "29-PCI 1", 73),
		temperature("40", "23-HD Controller-I/O controller", 69),
		temperature("41", "23-HD Controller-I/O controller", 54),
		temperature("42", "23-HD Controller-I/O controller", 56),
		temperature("43", "29-PCI 1-I/O module", 71),
		temperature("44", "29-PCI 1-I/O module", 54),
		`{"MemberId": "45", "Name": "29-PCI 1-I/O module", "Status": {"State": "Absent"}}`,
	}, ",") + `]}`

	sensors := map[string]string{
		"Chassis/1/Sensors": `{"Members": [` + strings.Join([]string{
			`{"@odata.id": "/redfish/v1/Chassis/1/Sensors/22"}`, `{"@odata.id": "/redfish/v1/Chassis/1/Sensors/28"}`,
			`{"@odata.id": "/redfish/v1/Chassis/1/Sensors/40"}`, `{"@odata.id": "/redfish/v1/Chassis/1/Sensors/41"}`,
			`{"@odata.id": "/redfish/v1/Chassis/1/Sensors/42"}`, `{"@odata.id": "/redfish/v1/Chassis/1/Sensors/43"}`,
			`{"@odata.id": "/redfish/v1/Chassis/1/Sensors/44"}`,
		}, ",") + `]}`,
		"Chassis/1/Sensors/22": sensor("22", "23-HD Controller", 76),
		"Chassis/1/Sensors/28": sensor("28", "29-PCI 1", 73),
		"Chassis/1/Sensors/40": sensor("40", "23-HD Controller-I/O controller", 69),
		"Chassis/1/Sensors/41": sensor("41", "23-HD Controller-I/O controller", 54),
		"Chassis/1/Sensors/42": sensor("42", "23-HD Controller-I/O controller", 56),
		"Chassis/1/Sensors/43": sensor("43", "29-PCI 1-I/O module", 71),
		"Chassis/1/Sensors/44": sensor("44", "29-PCI 1-I/O module", 54),
	}

	tests := map[string]map[string]string{
		"thermal": chassisMockup(`"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"}`, map[string]string{
			"Chassis/1/Thermal": thermal,
		}),
		"sensors": chassisMockup(`"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}`, sensors),
	}

	for name, mockup := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			srv := fakeilo.New(t, writeMockup(t, mockup))
			e := newTestExporter(t, withExporter(func(e *exporter) {
				e.logger = promslog.New(&promslog.Config{Writer: &buf})
			}))

			got := temperatures(scrape(t, e, srv.Host()))

			if !slices.Equal(got, want) {
				t.Errorf("got temperatures:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			if out := buf.String(); strings.Contains(out, "Invalid metrics") {
				t.Errorf("expected no dropped metrics, got:\n%s", out)
			}
		})
	}
}

// TestLogsDroppedMetricsOnce makes sure metrics dropped by the registry (e.g. duplicates) are only logged on change
func TestLogsDroppedMetricsOnce(t *testing.T) {
	power := func(serials ...string) string {
		var supplies []string
		for _, s := range serials {
			supplies = append(supplies, fmt.Sprintf(`{"SerialNumber": %q, "Status": {"Health": "OK", "State": "Enabled"}}`, s))
		}
		return `{"PowerSupplies": [` + strings.Join(supplies, ",") + `]}`
	}

	dir := writeMockup(t, chassisMockup(`"Power": {"@odata.id": "/redfish/v1/Chassis/1/Power"}`, map[string]string{
		"Chassis/1/Power": power("ABC123", "ABC123"),
	}))
	srv := fakeilo.New(t, dir)

	var buf bytes.Buffer
	e := newTestExporter(t, withExporter(func(e *exporter) {
		e.logger = promslog.New(&promslog.Config{Writer: &buf})
	}))

	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())
	if n := strings.Count(buf.String(), `msg="Invalid metrics dropped"`); n != 1 {
		t.Errorf("expected a single warning for dropped metrics, got %d:\n%s", n, buf.String())
	}
	buf.Reset()

	if err := os.WriteFile(filepath.Join(dir, "Chassis", "1", "Power", "index.json"), []byte(power("ABC123", "DEF456")), 0o600); err != nil {
		t.Fatal(err)
	}
	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())
	if out := buf.String(); strings.Count(out, `msg="Invalid metrics resolved"`) != 1 || strings.Count(out, "\n") != 1 {
		t.Errorf("expected a single info message when resolved, got:\n%s", out)
	}
}
