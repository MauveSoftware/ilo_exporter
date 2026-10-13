// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
)

func TestInvalidCredentialsStopScrape(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"), fakeilo.WithCredentials(testUser, "other"))

	body := scrape(t, newTestExporter(t), srv.Host())

	if n := len(srv.Requests()); n != 1 {
		t.Errorf("expected a single request (no further failed logins), got %d:\n%s", n, strings.Join(srv.Requests(), "\n"))
	}

	for _, want := range []string{
		`ilo_up{host="` + srv.Host() + `"} 0`,
		`ilo_scrape_collector_success{collector="memory",host="` + srv.Host() + `"} 0`,
		`ilo_scrape_redfish_requests{host="` + srv.Host() + `"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s, got:\n%s", want, body)
		}
	}
}

func TestUnreachableHost(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := l.Addr().String()
	_ = l.Close()

	body := scrape(t, newTestExporter(t), host)

	for _, want := range []string{
		`ilo_up{host="` + host + `"} 0`,
		`ilo_scrape_redfish_requests{host="` + host + `"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s, got:\n%s", want, body)
		}
	}
}

func TestCollectParameter(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"))
	e := newTestExporter(t)
	scrape(t, e, srv.Host())
	srv.ResetRequests()

	r := httptest.NewRequest(http.MethodGet, "/metrics?host="+srv.Host()+"&collect[]=power&collect[]=manager", nil)
	body := scrapeRequest(t, e, r)

	want := []string{"GET /redfish/v1/Chassis/1/Power", "GET /redfish/v1/Managers/1", "GET /redfish/v1/Systems/1"}
	if got := srv.Requests(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if strings.Contains(body, "ilo_memory_") || strings.Contains(body, "ilo_system_scrape_duration_second") {
		t.Errorf("expected no metrics of disabled collectors, got:\n%s", body)
	}
	if !strings.Contains(body, "ilo_power_current_watt") || !strings.Contains(body, "ilo_manager_info") {
		t.Errorf("expected metrics of enabled collectors, got:\n%s", body)
	}

	r = httptest.NewRequest(http.MethodGet, "/metrics?host="+srv.Host()+"&collect[]=foo", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown collector, got %d", w.Code)
	}
}
