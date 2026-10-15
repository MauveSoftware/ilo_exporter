// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// loginTimeout limits a login, which is shared by all scrapes of a host
const loginTimeout = 15 * time.Second

// Authenticator authenticates requests
type Authenticator interface {
	// Apply adds the credentials to a request
	Apply(ctx context.Context, req *http.Request) error

	// Unauthorized is called if a request was rejected with 401. It returns true if the request should be retried.
	Unauthorized(req *http.Request) bool
}

// BasicAuth authenticates every request using HTTP basic authentication.
// iLO creates and deletes a session for every request authenticated this way.
type BasicAuth struct {
	Username string
	Password string
}

// Apply implements Authenticator
func (a *BasicAuth) Apply(_ context.Context, req *http.Request) error {
	req.SetBasicAuth(a.Username, a.Password)
	return nil
}

// Unauthorized implements Authenticator
func (a *BasicAuth) Unauthorized(*http.Request) bool {
	return false
}

// SessionAuth authenticates requests using a Redfish session, which is created on first use and
// created again if it expired. A SessionAuth should be used for all requests to a host.
// Sessions are limited resources of an iLO, so Close must be called when the session is not needed anymore.
type SessionAuth struct {
	sessionsURL string
	username    string
	password    string
	client      *http.Client
	logger      *slog.Logger
	group       singleflight.Group

	mu       sync.Mutex
	token    string
	location string
}

// NewSessionAuth creates an authenticator using sessions of the Redfish service of host (host[:port])
func NewSessionAuth(host, username, password string, client *http.Client, logger *slog.Logger) *SessionAuth {
	return &SessionAuth{
		sessionsURL: fmt.Sprintf("https://%s/redfish/v1/SessionService/Sessions", host),
		username:    username,
		password:    password,
		client:      client,
		logger:      logger,
	}
}

// Apply implements Authenticator
func (a *SessionAuth) Apply(ctx context.Context, req *http.Request) error {
	a.mu.Lock()
	token := a.token
	a.mu.Unlock()

	if token == "" {
		var err error
		token, err = a.login(ctx)
		if err != nil {
			return err
		}
	}

	req.Header.Set("X-Auth-Token", token)
	return nil
}

// Unauthorized implements Authenticator. The session used by the request is discarded, so the request
// is retried with a new session. Sessions created by concurrent requests in the meantime are kept.
func (a *SessionAuth) Unauthorized(req *http.Request) bool {
	used := req.Header.Get("X-Auth-Token")

	a.mu.Lock()
	defer a.mu.Unlock()

	if used != "" && a.token == used {
		a.logger.Debug("Session expired")
		a.token = ""
		a.location = ""
	}

	return used != ""
}

// login creates a session. Concurrent logins are shared and not canceled with ctx.
func (a *SessionAuth) login(ctx context.Context) (string, error) {
	ch := a.group.DoChan("login", func() (any, error) {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loginTimeout)
		defer cancel()

		token, location, err := a.createSession(lctx)
		if err != nil {
			return "", err
		}

		a.mu.Lock()
		a.token = token
		a.location = location
		a.mu.Unlock()

		a.logger.Debug("Session created")
		return token, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return "", res.Err
		}
		return res.Val.(string), nil
	case <-ctx.Done():
		return "", fmt.Errorf("login: %w", context.Cause(ctx))
	}
}

func (a *SessionAuth) createSession(ctx context.Context) (token, location string, err error) {
	body, err := json.Marshal(map[string]string{
		"UserName": a.username,
		"Password": a.password,
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.sessionsURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("OData-Version", "4.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", newHTTPError(resp, a.sessionsURL)
	}

	token = resp.Header.Get("X-Auth-Token")
	if token == "" {
		return "", "", errors.New("no session token returned by login")
	}

	return token, a.sessionURL(resp.Header.Get("Location")), nil
}

// sessionURL resolves the location of a session. Locations of other hosts are ignored,
// so the token is never sent to another host.
func (a *SessionAuth) sessionURL(location string) string {
	base, err := url.Parse(a.sessionsURL)
	if err != nil || location == "" {
		return ""
	}

	u, err := base.Parse(location)
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host {
		return ""
	}

	return u.String()
}

// Close deletes the session (if any)
func (a *SessionAuth) Close(ctx context.Context) error {
	a.mu.Lock()
	token, location := a.token, a.location
	a.token, a.location = "", ""
	a.mu.Unlock()

	if token == "" || location == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, location, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", token)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not delete session: %w", err)
	}
	defer drainAndClose(resp)

	// the session might have expired already
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("could not delete session: %s", resp.Status)
	}

	a.logger.Debug("Session deleted")
	return nil
}

// drainAndClose reads the remaining body (so the connection can be reused) and closes it
func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
