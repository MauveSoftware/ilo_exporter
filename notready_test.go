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
	e := newTestExporter(t)

	start := time.Now()
	got := scrape(t, e, srv.Host())
	elapsed := time.Since(start)

	got = normalizeRequestCount(strings.ReplaceAll(got, srv.Host(), "ilo.test"))
	if got != want {
		t.Errorf("metrics differ from a scrape without retry:\n%s", diff(want, got))
	}

	if n := len(requestsOf(srv, "/Storage/DA000002")); n != 2 {
		t.Errorf("expected the resource to be requested twice, got %d", n)
	}
	if elapsed < time.Second {
		t.Errorf("expected the retry to wait the delay requested by the iLO (1s), scrape took %v", elapsed)
	}

	srv.ResetRequests()
	scrape(t, e, srv.Host())
	if len(requestsOf(srv, "$expand")) == 0 {
		t.Error("expected $expand to be used further: a resource not being ready is no rejection of $expand")
	}
}

func TestResourceNotReadyGivesUp(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo7-dl380a"), fakeilo.WithNotReady(notReadyPath, -1, 1))
	e := newTestExporter(t)

	body := scrape(t, e, srv.Host())

	if !strings.Contains(body, `ilo_scrape_collector_success{collector="storage",host="`+srv.Host()+`"} 0`) {
		t.Errorf("expected storage collector to fail, got:\n%s", body)
	}
	if !strings.Contains(body, "ilo_storage_disk_healthy") {
		t.Error("expected drives of the other storage subsystems to be reported")
	}
	if n := len(requestsOf(srv, "/Storage/DA000002")); n != 3 {
		t.Errorf("expected the resource to be requested 3 times (2 retries), got %d", n)
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
	if n := len(requestsOf(srv, "/Storage/DA000002")); n != 1 {
		t.Errorf("expected no retry which can not complete before the deadline, got %d requests", n)
	}
}

// scrapeFixture returns the metrics of a scrape of a fixture without errors, with the host replaced by HOST
func scrapeFixture(t *testing.T, gen string) string {
	t.Helper()

	srv := fakeilo.New(t, mockupDir(gen))
	return strings.ReplaceAll(scrape(t, newTestExporter(t), srv.Host()), srv.Host(), "HOST")
}
