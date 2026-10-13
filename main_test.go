// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
	"github.com/sirupsen/logrus"
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
	logrus.SetOutput(io.Discard)
	*username = testUser
	*password = testPassword
	os.Exit(m.Run())
}

func TestGolden(t *testing.T) {
	for gen := range generations {
		t.Run(gen, func(t *testing.T) {
			srv := fakeilo.New(t, mockupDir(gen), fakeilo.WithCredentials(testUser, testPassword))

			body := scrape(t, srv.Host())

			checkGolden(t, gen+".prom", normalizeMetrics(body, srv.Host()))
			checkGolden(t, gen+".requests", strings.Join(srv.Requests(), "\n")+"\n")
		})
	}
}

func mockupDir(gen string) string {
	return filepath.Join("testdata", "mockups", gen)
}

func scrape(t *testing.T, host string) string {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/metrics?host="+host, nil)
	w := httptest.NewRecorder()
	errorHandler(handleMetricsRequest).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}

	return w.Body.String()
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
