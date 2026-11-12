// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
)

// redirectingILO returns an iLO redirecting every request to another server, which counts the requests it receives
func redirectingILO(t *testing.T, status int) (ilo *httptest.Server, redirected *atomic.Int64) {
	t.Helper()

	redirected = &atomic.Int64{}
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		t.Errorf("redirect followed: %s %s (X-Auth-Token %q, Authorization %q)",
			r.Method, r.URL, r.Header.Get("X-Auth-Token"), r.Header.Get("Authorization"))
		w.Header().Set("X-Auth-Token", "token")
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(other.Close)

	ilo = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, status)
	}))
	t.Cleanup(ilo.Close)

	return ilo, redirected
}

func TestNewHTTPClientDoesNotFollowRedirectsOfGet(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ilo, redirected := redirectingILO(t, status)

			cl := NewClient(strings.TrimPrefix(ilo.URL, "https://"), "user", "password", noop.NewTracerProvider().Tracer(""),
				WithHTTPClient(NewHTTPClient(ilo.Client().Transport)))
			err := cl.Get(context.Background(), "/redfish/v1/Systems/1/", &struct{}{})

			var he *HTTPError
			if !errors.As(err, &he) || he.StatusCode != status {
				t.Errorf("expected HTTPError with status %d, got %v", status, err)
			}
			if n := redirected.Load(); n != 0 {
				t.Errorf("expected no request to the redirect target, got %d", n)
			}
		})
	}
}

func TestNewHTTPClientDoesNotFollowRedirectsOfLogin(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ilo, redirected := redirectingILO(t, status)

			auth := NewSessionAuth(strings.TrimPrefix(ilo.URL, "https://"), "user", "password",
				NewHTTPClient(ilo.Client().Transport), slog.New(slog.DiscardHandler))
			req, _ := http.NewRequest(http.MethodGet, ilo.URL, nil)
			err := auth.Apply(context.Background(), req)

			if err == nil {
				t.Error("expected login to fail on redirect")
			}
			if n := redirected.Load(); n != 0 {
				t.Errorf("expected no request (and credentials) sent to the redirect target, got %d", n)
			}
		})
	}
}
