// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
	"github.com/prometheus/common/promslog"
)

func countRequests(srv *fakeilo.Server, req string) int {
	n := 0
	for _, r := range srv.Requests() {
		if r == req {
			n++
		}
	}

	return n
}

func TestDiscoveryIsSharedByConcurrentScrapes(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo6"), fakeilo.WithLatency(10*time.Millisecond))
	e := newTestExporter(t)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { scrape(t, e, srv.Host()) })
	}
	wg.Wait()

	if n := countRequests(srv, "GET /redfish/v1/"); n != 1 {
		t.Errorf("expected a single discovery, got %d requests of the service root", n)
	}
}

func TestStaleTopologyIsDiscoveredAgain(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo6"))
	e := newTestExporter(t)

	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())
	if n := countRequests(srv, "GET /redfish/v1/"); n != 1 {
		t.Fatalf("expected a single discovery, got %d", n)
	}

	srv.SetStatus("/redfish/v1/Chassis/1/Power", http.StatusNotFound)
	scrape(t, e, srv.Host())
	srv.SetStatus("/redfish/v1/Chassis/1/Power", 0)
	scrape(t, e, srv.Host())

	if n := countRequests(srv, "GET /redfish/v1/"); n != 2 {
		t.Errorf("expected discovery after resource of topology was not found, got %d discoveries", n)
	}
}

func TestDiscoveryWithoutCache(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo7"))
	e := newTestExporter(t, withExporter(func(e *exporter) { e.topologies = nil }))

	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())

	if n := countRequests(srv, "GET /redfish/v1/"); n != 2 {
		t.Errorf("expected discovery on every scrape, got %d", n)
	}
}

func TestLogsStateChangesOnly(t *testing.T) {
	var buf bytes.Buffer
	srv := fakeilo.New(t, mockupDir("ilo7"))
	e := newTestExporter(t, withExporter(func(e *exporter) {
		e.logger = promslog.New(&promslog.Config{Writer: &buf})
	}))

	msgRegex := regexp.MustCompile(`msg="([^"]*)"|msg=(\S+)`)
	logged := func() []string {
		var msgs []string
		for _, m := range msgRegex.FindAllStringSubmatch(buf.String(), -1) {
			msgs = append(msgs, m[1]+m[2])
		}
		buf.Reset()
		return msgs
	}

	scrape(t, e, srv.Host())
	if msgs := logged(); len(msgs) != 0 {
		t.Errorf("expected no log output, got %v", msgs)
	}

	srv.SetStatus("/redfish/v1/Chassis/1/Thermal", http.StatusInternalServerError)
	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())
	if msgs := logged(); !slices.Equal(msgs, []string{"Scrape incomplete"}) {
		t.Errorf("expected a single warning for an incomplete scrape, got %v", msgs)
	}

	srv.SetStatus("/redfish/v1/Chassis/1/Thermal", 0)
	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())
	if msgs := logged(); !slices.Equal(msgs, []string{"Scrape complete"}) {
		t.Errorf("expected a single info on recovery, got %v", msgs)
	}

	srv.SetStatus("/redfish/v1/Systems/1", http.StatusUnauthorized)
	scrape(t, e, srv.Host())
	scrape(t, e, srv.Host())
	if msgs := logged(); !slices.Equal(msgs, []string{"iLO down"}) {
		t.Errorf("expected a single warning for an iLO which is down, got %v", msgs)
	}

	srv.SetStatus("/redfish/v1/Systems/1", 0)
	scrape(t, e, srv.Host())
	if msgs := logged(); !slices.Equal(msgs, []string{"iLO up"}) {
		t.Errorf("expected a single info when reachable again, got %v", msgs)
	}
}
