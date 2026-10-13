// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace/noop"
)

func newTestClient(t *testing.T, srv *fakeilo.Server) *client.APIClient {
	t.Helper()

	tr, err := client.NewTransport(client.TransportConfig{MaxConnsPerHost: 4, InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.CloseIdleConnections)

	return client.NewClient(srv.Host(), "", "", noop.NewTracerProvider().Tracer(""), client.WithHTTPClient(&http.Client{Transport: tr}))
}

// TestDescribedMetrics checks with a pedantic registry that all collected metrics are described consistently
func TestDescribedMetrics(t *testing.T) {
	for _, gen := range []string{"ilo5", "ilo6", "ilo7"} {
		t.Run(gen, func(t *testing.T) {
			srv := fakeilo.New(t, filepath.Join("..", "..", "testdata", "mockups", gen))

			reg := prometheus.NewPedanticRegistry()
			reg.MustRegister(New(context.Background(), newTestClient(t, srv), noop.NewTracerProvider().Tracer("")))

			mfs, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			if len(mfs) == 0 {
				t.Fatal("no metrics collected")
			}
		})
	}
}

func TestLegacyMemoryDIMM(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantValid   bool
		wantHealthy float64
		wantSizeMB  uint64
	}{
		{
			name:        "iLO 4 good",
			json:        `{"Name": "PROC 1 DIMM 1", "DIMMStatus": "GoodInUse", "SizeMB": 16384}`,
			wantValid:   true,
			wantHealthy: 1,
			wantSizeMB:  16384,
		},
		{
			name:      "iLO 4 unknown",
			json:      `{"Name": "PROC 1 DIMM 2", "DIMMStatus": "Unknown", "SizeMB": 0}`,
			wantValid: false,
		},
		{
			name:        "iLO 5 degraded via OEM status",
			json:        `{"Name": "proc1dimm1", "Status": {"State": "Enabled"}, "Oem": {"Hpe": {"DIMMStatus": "Degraded"}}, "CapacityMiB": 32768}`,
			wantValid:   true,
			wantHealthy: 0,
			wantSizeMB:  32768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := MemoryDIMM{}
			if err := json.Unmarshal([]byte(tt.json), &d); err != nil {
				t.Fatal(err)
			}

			if got := d.IsValid(); got != tt.wantValid {
				t.Fatalf("IsValid() = %v, want %v", got, tt.wantValid)
			}
			if !tt.wantValid {
				return
			}
			if got := d.HealthValue(); got != tt.wantHealthy {
				t.Errorf("HealthValue() = %v, want %v", got, tt.wantHealthy)
			}
			if got := d.SizeMB(); got != tt.wantSizeMB {
				t.Errorf("SizeMB() = %v, want %v", got, tt.wantSizeMB)
			}
		})
	}
}

func TestLegacyFan(t *testing.T) {
	f := Fan{}
	if err := json.Unmarshal([]byte(`{"FanName": "Fan 1", "CurrentReading": 23, "Status": {"Health": "OK", "State": "Enabled"}}`), &f); err != nil {
		t.Fatal(err)
	}

	if f.Name() != "Fan 1" || f.Reading() != 23 {
		t.Errorf("got name %q and reading %v, want Fan 1 and 23", f.Name(), f.Reading())
	}
}

func TestDriveLocation(t *testing.T) {
	tests := map[string]string{
		`{"Location": "1I:1:1"}`:                                                                       "1I:1:1",
		`{"Location": [{"Info": "1:1", "InfoFormat": "Box:Bay"}]}`:                                     "1:1",
		`{"PhysicalLocation": {"PartLocation": {"ServiceLabel": "Slot=12:Port=1I:Box=1:Bay=1"}}}`:      "Slot=12:Port=1I:Box=1:Bay=1",
		`{"Location": null, "PhysicalLocation": {"PartLocation": {"ServiceLabel": "Embedded:Bay=2"}}}`: "Embedded:Bay=2",
	}

	for in, want := range tests {
		d := DiskDrive{}
		if err := json.Unmarshal([]byte(in), &d); err != nil {
			t.Fatal(err)
		}

		if got := d.GetLocation(); got != want {
			t.Errorf("%s: got location %q, want %q", in, got, want)
		}
	}
}

func TestDriveCapacity(t *testing.T) {
	smart := DiskDrive{CapacityMB: 457862}
	if got := smart.CapacityBytes(); got != 457862*1024*1024 {
		t.Errorf("SmartStorage capacity: got %v, want %v", got, 457862*1024*1024)
	}

	dmtf := DiskDrive{CapacityB: 480103981056}
	if got := dmtf.CapacityBytes(); got != 480103981056 {
		t.Errorf("DMTF capacity: got %v, want %v", got, 480103981056)
	}
}

func TestAbsentDIMM(t *testing.T) {
	for _, in := range []string{
		`{"Name": "proc1dimm2", "Status": {"Health": "OK", "State": "Absent"}, "Oem": {"Hpe": {"DIMMStatus": "NotPresent"}}, "CapacityMiB": 0}`,
		`{"Name": "proc1dimm2", "Oem": {"Hpe": {"DIMMStatus": "NotPresent"}}}`,
	} {
		d := MemoryDIMM{}
		if err := json.Unmarshal([]byte(in), &d); err != nil {
			t.Fatal(err)
		}

		if d.IsValid() {
			t.Errorf("expected empty slot to be invalid: %s", in)
		}
	}
}
