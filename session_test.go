// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/fakeilo"
)

func withSessions() testOption {
	return withExporter(func(e *exporter) { e.enableSessions(100) })
}

func TestSessionAuth(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo6"), fakeilo.WithCredentials(testUser, testPassword), fakeilo.WithoutBasicAuth())
	e := newTestExporter(t, withSessions())

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			body := scrape(t, e, srv.Host())
			if !strings.Contains(body, `ilo_up{host="`+srv.Host()+`"} 1`) {
				t.Errorf("expected scrape using session to succeed, got:\n%s", body)
			}
		})
	}
	wg.Wait()
	scrape(t, e, srv.Host())

	if n := srv.Logins(); n != 1 {
		t.Errorf("expected a single login for all scrapes, got %d", n)
	}

	e.close(context.Background())
	if n := srv.Sessions(); n != 0 {
		t.Errorf("expected session to be deleted on close, got %d open sessions", n)
	}
}

func TestSessionRenewal(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo7"), fakeilo.WithCredentials(testUser, testPassword), fakeilo.WithoutBasicAuth())
	e := newTestExporter(t, withSessions())

	scrape(t, e, srv.Host())
	srv.ExpireSessions()

	body := scrape(t, e, srv.Host())
	if !strings.Contains(body, `ilo_up{host="`+srv.Host()+`"} 1`) {
		t.Errorf("expected scrape to succeed with renewed session, got:\n%s", body)
	}

	if n := srv.Logins(); n != 2 {
		t.Errorf("expected a second login after the session expired, got %d logins", n)
	}

	e.close(context.Background())
	if n := srv.Sessions(); n != 0 {
		t.Errorf("expected session to be deleted on close, got %d open sessions", n)
	}
}

func TestSessionInvalidCredentials(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"), fakeilo.WithCredentials(testUser, "other"))
	e := newTestExporter(t, withSessions())

	body := scrape(t, e, srv.Host())

	if !strings.Contains(body, `ilo_up{host="`+srv.Host()+`"} 0`) {
		t.Errorf("expected ilo_up 0, got:\n%s", body)
	}

	if got := srv.Requests(); len(got) != 1 || got[0] != "POST /redfish/v1/SessionService/Sessions" {
		t.Errorf("expected a single login attempt, got:\n%s", strings.Join(got, "\n"))
	}
}

func TestSessionsDeletedOnEviction(t *testing.T) {
	srv := fakeilo.New(t, mockupDir("ilo5"))
	other := fakeilo.New(t, mockupDir("ilo5"))
	e := newTestExporter(t, withExporter(func(e *exporter) { e.enableSessions(1) }))

	scrape(t, e, srv.Host())
	scrape(t, e, other.Host())

	deadline := time.Now().Add(2 * time.Second)
	for srv.Sessions() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if n := srv.Sessions(); n != 0 {
		t.Errorf("expected session of evicted host to be deleted, got %d open sessions", n)
	}
	if n := other.Sessions(); n != 1 {
		t.Errorf("expected session of cached host to be kept, got %d open sessions", n)
	}

	e.close(context.Background())
}
