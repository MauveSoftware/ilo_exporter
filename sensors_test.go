// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
