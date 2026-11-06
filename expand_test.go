// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
)

// TestExpandEquivalence makes sure the metrics do not depend on the support of $expand
func TestExpandEquivalence(t *testing.T) {
	tests := map[string]struct {
		srvOpts []fakeilo.Option
		expOpts []testOption
	}{
		"expand ignored":  {srvOpts: []fakeilo.Option{fakeilo.WithoutExpand()}},
		"expand rejected": {srvOpts: []fakeilo.Option{fakeilo.WithExpandStatus(http.StatusBadRequest)}},
		"expand disabled": {expOpts: []testOption{withExporter(func(e *exporter) { e.noExpand = true })}},
	}

	for gen := range generations {
		for name, tt := range tests {
			t.Run(gen+"/"+name, func(t *testing.T) {
				expanded := fakeilo.New(t, mockupDir(gen))
				want := scrape(t, newTestExporter(t), expanded.Host())

				srv := fakeilo.New(t, mockupDir(gen), tt.srvOpts...)
				got := scrape(t, newTestExporter(t, tt.expOpts...), srv.Host())

				want = normalizeRequestCount(strings.ReplaceAll(want, expanded.Host(), "ilo.test"))
				got = normalizeRequestCount(strings.ReplaceAll(got, srv.Host(), "ilo.test"))
				if got != want {
					t.Errorf("metrics differ from metrics retrieved using $expand:\n%s", diff(want, got))
				}
			})
		}
	}
}

func TestExpandRejectedIsNotRetried(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"), fakeilo.WithExpandStatus(http.StatusBadRequest))
	e := newTestExporter(t)

	scrape(t, e, srv.Host())
	srv.ResetRequests()
	scrape(t, e, srv.Host())

	for _, r := range srv.Requests() {
		if strings.Contains(r, "$expand") {
			t.Errorf("expected no further requests using $expand after rejection, got %s", r)
		}
	}
}

func TestPagination(t *testing.T) {
	dimm := func(name string) string {
		return `{"@odata.type": "#Memory.v1_7_1.Memory", "Name": "` + name + `", "Status": {"Health": "OK", "State": "Enabled"}, "CapacityMiB": 1024}`
	}
	dir := writeMockup(t, map[string]string{
		"":                       `{"Systems": {"@odata.id": "/redfish/v1/Systems"}, "ProtocolFeaturesSupported": {"ExpandQuery": {"NoLinks": true}}}`,
		"Systems":                `{"Members": [{"@odata.id": "/redfish/v1/Systems/1"}]}`,
		"Systems/1":              `{"PowerState": "On", "Memory": {"@odata.id": "/redfish/v1/Systems/1/Memory"}}`,
		"Systems/1/Memory":       `{"Members": [{"@odata.id": "/redfish/v1/Systems/1/Memory/dimm1"}], "Members@odata.nextLink": "/redfish/v1/Systems/1/MemoryPage2"}`,
		"Systems/1/MemoryPage2":  `{"Members": [{"@odata.id": "/redfish/v1/Systems/1/Memory/dimm2"}]}`,
		"Systems/1/Memory/dimm1": dimm("dimm1"),
		"Systems/1/Memory/dimm2": dimm("dimm2"),
	})
	srv := fakeilo.New(t, dir)

	body := scrape(t, newTestExporter(t), srv.Host())

	for _, name := range []string{"dimm1", "dimm2"} {
		if !strings.Contains(body, `ilo_memory_dimm_byte{host="`+srv.Host()+`",name="`+name+`"} 1.073741824e+09`) {
			t.Errorf("expected metrics of %s, got:\n%s", name, body)
		}
	}
}
