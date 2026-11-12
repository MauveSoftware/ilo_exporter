// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
)

func TestHTTPErrorContainsMessageID(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"extended info": {
			body: `{"error": {"code": "iLO.0.10.ExtendedInfo", "message": "See @Message.ExtendedInfo for more information.", "@Message.ExtendedInfo": [{"MessageId": "Base.1.4.ResourceMissingAtURI"}]}}`,
			want: "returned 400 Bad Request (Base.1.4.ResourceMissingAtURI)",
		},
		"extended info with args": {
			body: `{"error": {"code": "iLO.0.10.ExtendedInfo", "@Message.ExtendedInfo": [{"MessageId": "Base.1.4.QueryNotSupported", "MessageArgs": ["$expand"]}]}}`,
			want: "returned 400 Bad Request (Base.1.4.QueryNotSupported: $expand)",
		},
		"code only": {
			body: `{"error": {"code": "Base.1.4.GeneralError", "message": "something failed"}}`,
			want: "returned 400 Bad Request (Base.1.4.GeneralError)",
		},
		"no JSON": {
			body: `<html>Bad Request</html>`,
			want: "returned 400 Bad Request",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			cl := NewClient(strings.TrimPrefix(srv.URL, "https://"), "", "", noop.NewTracerProvider().Tracer(""), WithHTTPClient(srv.Client()))
			err := cl.Get(context.Background(), "/redfish/v1/Systems/1/Storage/DA000002/", &struct{}{})

			var he *HTTPError
			if !errors.As(err, &he) || he.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected HTTPError with status 400, got %v", err)
			}
			if !strings.HasSuffix(err.Error(), tt.want) {
				t.Errorf("got %q, want suffix %q", err.Error(), tt.want)
			}
		})
	}
}
