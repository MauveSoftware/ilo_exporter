// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
)

// notReadyPath is the storage of an NVMe drive managed by iLO. iLO 5 answers requests for such resources with
// iLO.2.15.ResourceNotReadyRetry until it retrieved the data from the drive.
const notReadyPath = "/redfish/v1/Systems/1/Storage/DA000002"

func requestsOf(srv *fakeilo.Server, part string) []string {
	var res []string
	for _, r := range srv.Requests() {
		if strings.Contains(r, part) {
			res = append(res, r)
		}
	}

	return res
}

func TestResourceNotReadyIsRetried(t *testing.T) {
	want := normalizeRequestCount(strings.ReplaceAll(scrapeFixture(t, "ilo7-dl380a"), "HOST", "ilo.test"))

	srv := fakeilo.New(t, mockupDir("ilo7-dl380a"), fakeilo.WithNotReady(notReadyPath, 1, 1))
	e := newTestExporter(t, withExporter(func(e *exporter) { e.noExpand = true }))

	start := time.Now()
	got := scrape(t, e, srv.Host())
	elapsed := time.Since(start)

	got = normalizeRequestCount(strings.ReplaceAll(got, srv.Host(), "ilo.test"))
	if got != want {
		t.Errorf("metrics differ from a scrape without retry:\n%s", diff(want, got))
	}

	if n := len(requestsOf(srv, "/Storage/DA000002/Drives")); n != 1 {
		t.Errorf("expected the drive to be requested once, got %d", n)
	}
	if n := len(requestsOf(srv, "/Storage/DA000002")) - 1; n != 2 {
		t.Errorf("expected the storage to be requested twice, got %d", n)
	}
	if elapsed < time.Second {
		t.Errorf("expected the retry to wait the delay requested by the iLO (1s), scrape took %v", elapsed)
	}
}

// TestResourceNotReadyExpandFallback covers resources which are not ready when requested with $expand: iLO 5 keeps the
// data of storage of drives it manages fresh for a few seconds only, so all subordinate resources are rarely ready at
// the same time. Such resources are retrieved without $expand instead of retrying the request with $expand.
func TestResourceNotReadyExpandFallback(t *testing.T) {
	want := normalizeRequestCount(strings.ReplaceAll(scrapeFixture(t, "ilo7-dl380a"), "HOST", "ilo.test"))

	srv := fakeilo.New(t, mockupDir("ilo7-dl380a"), fakeilo.WithNotReady(notReadyPath, 1, 1))
	e := newTestExporter(t)

	start := time.Now()
	got := scrape(t, e, srv.Host())
	elapsed := time.Since(start)

	got = normalizeRequestCount(strings.ReplaceAll(got, srv.Host(), "ilo.test"))
	if got != want {
		t.Errorf("metrics differ from a scrape without retry:\n%s", diff(want, got))
	}

	wantRequests := []string{
		"GET /redfish/v1/Systems/1/Storage/DA000002",
		"GET /redfish/v1/Systems/1/Storage/DA000002/Drives/DA000002/",
		"GET /redfish/v1/Systems/1/Storage/DA000002?$expand=.",
	}
	if got := requestsOf(srv, "/Storage/DA000002"); strings.Join(got, "\n") != strings.Join(wantRequests, "\n") {
		t.Errorf("got requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantRequests, "\n"))
	}
	if elapsed > 900*time.Millisecond {
		t.Errorf("expected no wait for the request with $expand, scrape took %v", elapsed)
	}

	srv.ResetRequests()
	scrape(t, e, srv.Host())
	if len(requestsOf(srv, "$expand")) == 0 {
		t.Error("expected $expand to be used further: a resource not being ready is no rejection of $expand")
	}
}

func TestResourceNotReadyGivesUp(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo7-dl380a"), fakeilo.WithNotReady(notReadyPath, -1, 0))
	e := newTestExporter(t)

	body := scrape(t, e, srv.Host())

	if !strings.Contains(body, `ilo_scrape_collector_success{collector="storage",host="`+srv.Host()+`"} 0`) {
		t.Errorf("expected storage collector to fail, got:\n%s", body)
	}
	if !strings.Contains(body, "ilo_storage_disk_healthy") {
		t.Error("expected drives of the other storage subsystems to be reported")
	}
	// one request with $expand, one without and 3 retries
	if n := len(requestsOf(srv, "/Storage/DA000002")); n != 5 {
		t.Errorf("expected the resource to be requested 5 times, got %d", n)
	}

	srv.ResetRequests()
	scrape(t, e, srv.Host())
	if len(requestsOf(srv, "$expand")) == 0 {
		t.Error("expected $expand to be used further: a resource not being ready is no rejection of $expand")
	}
}

func TestResourceNotReadyRespectsDeadline(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo7-dl380a"), fakeilo.WithNotReady(notReadyPath, -1, 8))
	e := newTestExporter(t)

	r := httptest.NewRequest(http.MethodGet, "/metrics?host="+srv.Host(), nil)
	r.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", "3")

	start := time.Now()
	scrapeRequest(t, e, r)

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("expected no wait exceeding the scrape deadline, scrape took %v", elapsed)
	}
	// one request with $expand, one without, no retry
	if n := len(requestsOf(srv, "/Storage/DA000002")); n != 2 {
		t.Errorf("expected no retry which can not complete before the deadline, got %d requests", n)
	}
}

// scrapeFixture returns the metrics of a scrape of a fixture without errors, with the host replaced by HOST
func scrapeFixture(t *testing.T, gen string) string {
	t.Helper()

	srv := fakeilo.New(t, mockupDir(gen))
	return strings.ReplaceAll(scrape(t, newTestExporter(t), srv.Host()), srv.Host(), "HOST")
}
