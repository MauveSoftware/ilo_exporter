// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
	"github.com/MauveSoftware/ilo_exporter/internal/lru"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/MauveSoftware/ilo_exporter/pkg/topology"
	"github.com/prometheus/common/promslog"
)

var update = flag.Bool("update", false, "update golden files")

// generations maps the fixture name to the HPE mockup it was taken from
var generations = map[string]string{
	"ilo5": "DL360 Gen10 Plus, iLO 5 v3.11",
	"ilo6": "DL380a Gen11, iLO 6 v1.66",
	"ilo7": "DL360 Gen12, iLO 7 v1.14",
}

const (
	testUser     = "ilo_exporter"
	testPassword = "g3tM3trics"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

type testOption func(*exporter, *client.TransportConfig)

func withTransport(f func(*client.TransportConfig)) testOption {
	return func(_ *exporter, cfg *client.TransportConfig) {
		f(cfg)
	}
}

func withExporter(f func(*exporter)) testOption {
	return func(e *exporter, _ *client.TransportConfig) {
		f(e)
	}
}

func newTestExporter(t *testing.T, opts ...testOption) *exporter {
	t.Helper()

	cfg := client.TransportConfig{
		MaxConnsPerHost:    4,
		IdleConnTimeout:    time.Minute,
		InsecureSkipVerify: true,
	}
	e := &exporter{
		username:      testUser,
		password:      testPassword,
		logger:        promslog.NewNopLogger(),
		timeoutOffset: 500 * time.Millisecond,
		topologies:    topology.NewCache(100, time.Hour),
		states:        lru.New[hostState](100),
	}

	for _, o := range opts {
		o(e, &cfg)
	}

	tr, err := client.NewTransport(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.CloseIdleConnections)
	e.httpClient = &http.Client{Transport: tr}

	return e
}

func TestGolden(t *testing.T) {
	for gen := range generations {
		t.Run(gen, func(t *testing.T) {
			srv := fakeilo.New(t, mockupDir(gen), fakeilo.WithCredentials(testUser, testPassword))
			e := newTestExporter(t)

			first := scrape(t, e, srv.Host())
			discovery := srv.Requests()
			srv.ResetRequests()

			// the second scrape uses the cached topology (steady state)
			body := scrape(t, e, srv.Host())

			if normalizeRequestCount(first) != normalizeRequestCount(body) {
				t.Errorf("metrics of first and second scrape differ:\n%s", diff(normalizeRequestCount(first), normalizeRequestCount(body)))
			}

			checkGolden(t, gen+".prom", normalizeMetrics(body, srv.Host()))
			checkGolden(t, gen+".requests", strings.Join(srv.Requests(), "\n")+"\n")
			checkGolden(t, gen+".discovery.requests", strings.Join(discovery, "\n")+"\n")
		})
	}
}

func mockupDir(gen string) string {
	return filepath.Join("testdata", "mockups", gen)
}

func scrape(t *testing.T, e *exporter, host string) string {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/metrics?host="+host, nil)
	return scrapeRequest(t, e, r)
}

func scrapeRequest(t *testing.T, e *exporter, r *http.Request) string {
	t.Helper()

	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}

	return w.Body.String()
}

var requestCountLine = regexp.MustCompile(`(?m)^(ilo_scrape_redfish_requests(?:\{[^}]*\})?) .*$`)

// normalizeRequestCount removes the number of requests, which differs between scrapes with and without discovery
func normalizeRequestCount(body string) string {
	body = durationLine.ReplaceAllString(body, "$1 <duration>")
	return requestCountLine.ReplaceAllString(body, "$1 <requests>")
}

var durationLine = regexp.MustCompile(`(?m)^(ilo_\w*duration\w*(?:\{[^}]*\})?) .*$`)

// normalizeMetrics removes values that differ between test runs
func normalizeMetrics(body, host string) string {
	body = strings.ReplaceAll(body, host, "ilo.test")
	return durationLine.ReplaceAllString(body, "$1 <duration>")
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { // #nosec G306 -- test fixture
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatalf("could not read golden file (run with -update to create it): %v", err)
	}

	if got != string(want) {
		t.Errorf("%s differs from golden file (run with -update to accept):\n%s", name, diff(string(want), got))
	}
}

// diff returns a minimal line diff, good enough to spot changes in test output
func diff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")

	wantSet := make(map[string]int)
	for _, l := range wl {
		wantSet[l]++
	}
	gotSet := make(map[string]int)
	for _, l := range gl {
		gotSet[l]++
	}

	var b strings.Builder
	for _, l := range wl {
		if gotSet[l] > 0 {
			gotSet[l]--
			continue
		}
		b.WriteString("- " + l + "\n")
	}
	for _, l := range gl {
		if wantSet[l] > 0 {
			wantSet[l]--
			continue
		}
		b.WriteString("+ " + l + "\n")
	}
	return b.String()
}

func TestRejectsInvalidTargets(t *testing.T) {
	f, err := parseTargetFilter("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	e := newTestExporter(t, withExporter(func(e *exporter) { e.targets = f }))

	tests := map[string]int{
		"/metrics":                      http.StatusBadRequest,
		"/metrics?host=evil.example%23": http.StatusBadRequest,
		"/metrics?host=10.0.0.1%2Fx":    http.StatusBadRequest,
		"/metrics?host=192.168.0.1":     http.StatusForbidden,
		"/metrics?host=ilo.example.com": http.StatusForbidden,
	}

	for target, want := range tests {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)

		if w.Code != want {
			t.Errorf("%s: got status %d, want %d", target, w.Code, want)
		}
	}
}

// TestUnexpectedLinks makes sure links in unexpected places (or from a malicious server) do not crash the exporter
func TestUnexpectedLinks(t *testing.T) {
	dir := writeMockup(t, map[string]string{
		"Systems/1":        `{"PowerState": "On", "Memory": {"@odata.id": "/redfish/v1/Systems/1/Memory"}}`,
		"Systems/1/Memory": `{"Members": [{"@odata.id": "/redfish/v1/Elsewhere/dimm1"}, {"@odata.id": "x"}]}`,
		"Elsewhere/dimm1":  `{"Name": "dimm1", "Status": {"Health": "OK", "State": "Enabled"}, "CapacityMiB": 1024}`,
	})
	srv := fakeilo.New(t, dir)

	body := scrape(t, newTestExporter(t), srv.Host())

	if !strings.Contains(body, `ilo_memory_dimm_byte{host="`+srv.Host()+`",name="dimm1"} 1.073741824e+09`) {
		t.Errorf("expected metrics of DIMM with unexpected path, got:\n%s", body)
	}
}

// writeMockup creates a mockup directory from a map of resource path (relative to /redfish/v1) to JSON
func writeMockup(t *testing.T, resources map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for p, content := range resources {
		d := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "index.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestDefaultLogging(t *testing.T) {
	var buf bytes.Buffer
	e := newTestExporter(t, withExporter(func(e *exporter) {
		e.logger = promslog.New(&promslog.Config{Writer: &buf})
	}))

	srv := fakeilo.New(t, mockupDir("ilo7"), fakeilo.WithCredentials(testUser, testPassword))
	scrape(t, e, srv.Host())

	out := buf.String()
	if strings.Contains(out, "GET") || strings.Contains(out, "@odata") {
		t.Errorf("requests or responses logged at default level:\n%s", out)
	}

	if out != "" {
		t.Errorf("expected no log output for a successful scrape, got:\n%s", out)
	}
}
