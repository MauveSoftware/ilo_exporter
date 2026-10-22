// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
)

func TestConcurrencyLimitAcrossScrapes(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"), fakeilo.WithLatency(5*time.Millisecond))
	e := newTestExporter(t, withTransport(func(cfg *client.TransportConfig) { cfg.MaxConnsPerHost = 2 }))

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { scrape(t, e, srv.Host()) })
	}
	wg.Wait()

	if n := srv.MaxInFlight(); n > 2 {
		t.Errorf("expected at most 2 concurrent requests, got %d", n)
	}
}

func TestConnectionReuse(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"))
	e := newTestExporter(t)

	scrape(t, e, srv.Host())
	conns := srv.Connections()
	scrape(t, e, srv.Host())

	if n := srv.Connections(); n != conns {
		t.Errorf("expected connections to be reused by the second scrape, got %d new connections", n-conns)
	}

	if conns > 4 {
		t.Errorf("expected at most 4 connections (max concurrent requests), got %d", conns)
	}
}

func TestScrapeDeadline(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"), fakeilo.WithBlock("/redfish/v1/Chassis/1/Thermal"))
	e := newTestExporter(t)

	r := httptest.NewRequest(http.MethodGet, "/metrics?host="+srv.Host(), nil)
	r.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", "1.5")

	start := time.Now()
	body := scrapeRequest(t, e, r)
	elapsed := time.Since(start)

	if elapsed < 900*time.Millisecond || elapsed > 1500*time.Millisecond {
		t.Errorf("expected scrape to be canceled after 1s (timeout - offset), took %v", elapsed)
	}

	if !strings.Contains(body, "ilo_power_current_watt") {
		t.Errorf("expected metrics collected before the deadline to be returned, got:\n%s", body)
	}

	deadline := time.Now().Add(time.Second)
	for srv.InFlight() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := srv.InFlight(); n > 0 {
		t.Errorf("expected no requests in flight after the scrape, got %d", n)
	}
}

func TestScrapeContext(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		maxTimeout time.Duration
		want       time.Duration
	}{
		{name: "no limit", want: 0},
		{name: "max timeout only", maxTimeout: 30 * time.Second, want: 30 * time.Second},
		{name: "header", header: "10", want: 9500 * time.Millisecond},
		{name: "header below max timeout", header: "10", maxTimeout: 30 * time.Second, want: 9500 * time.Millisecond},
		{name: "max timeout below header", header: "60", maxTimeout: 30 * time.Second, want: 30 * time.Second},
		{name: "header below offset", header: "0.2", want: 200 * time.Millisecond},
		{name: "invalid header", header: "abc", maxTimeout: 30 * time.Second, want: 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.header != "" {
				r.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", tt.header)
			}

			ctx, cancel := scrapeContext(r, tt.maxTimeout, 500*time.Millisecond)
			defer cancel()

			d, ok := ctx.Deadline()
			if tt.want == 0 {
				if ok {
					t.Errorf("expected no deadline, got %v", time.Until(d))
				}
				return
			}

			if !ok {
				t.Fatal("expected deadline")
			}
			if got := time.Until(d); got > tt.want || got < tt.want-time.Second {
				t.Errorf("got timeout %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCertificateVerification(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"))

	e := newTestExporter(t, withTransport(func(cfg *client.TransportConfig) { cfg.InsecureSkipVerify = false }))
	if body := scrape(t, e, srv.Host()); strings.Contains(body, "ilo_power_current_watt") {
		t.Errorf("expected scrape to fail without trusted certificate, got:\n%s", body)
	}

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caFile, ca, 0o600); err != nil {
		t.Fatal(err)
	}

	e = newTestExporter(t, withTransport(func(cfg *client.TransportConfig) {
		cfg.InsecureSkipVerify = false
		cfg.CAFile = caFile
	}))
	if body := scrape(t, e, srv.Host()); !strings.Contains(body, "ilo_power_current_watt") {
		t.Errorf("expected scrape to succeed with CA file, got:\n%s", body)
	}
}
