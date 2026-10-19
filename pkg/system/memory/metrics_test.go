// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/MauveSoftware/ilo_exporter/pkg/common"
)

// fakeClient serves JSON bodies by path, relative to /redfish/v1/ like the API client
type fakeClient map[string]string

func (c fakeClient) HostName() string {
	return "ilo.example.com"
}

func (c fakeClient) Get(_ context.Context, path string, obj interface{}) error {
	body, ok := c[strings.Replace(path, "/redfish/v1/", "", 1)]
	if !ok {
		return fmt.Errorf("unexpected request for %s", path)
	}

	return json.Unmarshal([]byte(body), obj)
}

func TestCollectWithLinksOutsideSystems(t *testing.T) {
	dimm := `{"Name": "%s", "CapacityMiB": 16384, "Status": {"Health": "OK", "State": "Enabled"}}`
	cl := fakeClient{
		// a link without "Systems/" crashed the process before
		"Systems/1/Memory": `{"Members": [
			{"@odata.id": "/redfish/v1/Systems/1/Memory/proc1dimm1/"},
			{"@odata.id": "/redfish/v1/Memory/proc1dimm2/"}
		]}`,
		"Systems/1/Memory/proc1dimm1/": fmt.Sprintf(dimm, "proc1dimm1"),
		"Memory/proc1dimm2/":           fmt.Sprintf(dimm, "proc1dimm2"),
	}

	ch := make(chan prometheus.Metric, 10)
	cc := common.NewCollectorContext(context.Background(), cl, ch, noop.NewTracerProvider().Tracer("test"))

	cc.WaitGroup().Add(1)
	Collect("Systems/1", cc)
	cc.WaitGroup().Wait()
	close(ch)

	if n := cc.ErrCount(); n != 0 {
		t.Errorf("%d errors", n)
	}

	metrics := 0
	for range ch {
		metrics++
	}
	if metrics != 4 {
		t.Errorf("got %d metrics, want 4 (health and size of 2 DIMMs)", metrics)
	}
}
