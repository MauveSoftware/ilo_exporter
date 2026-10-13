// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// maxResponseSize limits the size of a response body read from the API
const maxResponseSize = 32 << 20

// HTTPError is returned when the API responds with a non-success status code
type HTTPError struct {
	StatusCode int
	URL        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s returned %d %s", e.URL, e.StatusCode, http.StatusText(e.StatusCode))
}

// IsFatal reports whether the error of a request indicates that further requests to the host
// will fail as well, e.g. because the host is unreachable or the credentials are invalid
func IsFatal(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.StatusCode == http.StatusUnauthorized || he.StatusCode == http.StatusForbidden
	}

	var ue *url.Error
	return errors.As(err, &ue) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// APIClient communicates with the API and parses the results
type APIClient struct {
	url        string
	hostName   string
	username   string
	password   string
	tracer     trace.Tracer
	client     *http.Client
	logger     *slog.Logger
	logBodies  bool
	requests   atomic.Int64
	failures   atomic.Int64
	firstErrMu sync.Mutex
	firstErr   error
}

// ClientOption applies options to APIClient
type ClientOption func(*APIClient)

// WithHTTPClient sets the HTTP client used for requests. The client (or its transport) should be
// shared between all API clients, so connections are reused and limits are enforced across scrapes.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *APIClient) {
		cl.client = c
	}
}

// WithLogger sets the logger used for request logging (debug level)
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *APIClient) {
		c.logger = logger
	}
}

// WithResponseLogging logs the body of every response (debug level)
func WithResponseLogging() ClientOption {
	return func(c *APIClient) {
		c.logBodies = true
	}
}

// NewClient creates a new client instance
func NewClient(hostName, username, password string, tracer trace.Tracer, opts ...ClientOption) *APIClient {
	cl := &APIClient{
		url:      fmt.Sprintf("https://%s/redfish/v1/", hostName),
		hostName: hostName,
		username: username,
		password: password,
		client:   http.DefaultClient,
		tracer:   tracer,
		logger:   slog.New(slog.DiscardHandler),
	}

	for _, o := range opts {
		o(cl)
	}

	return cl
}

// HostName returns the name of the host
func (cl *APIClient) HostName() string {
	return cl.hostName
}

// Requests returns the number of requests sent
func (cl *APIClient) Requests() int64 {
	return cl.requests.Load()
}

// Failures returns the number of failed requests and the first error which occurred
func (cl *APIClient) Failures() (int64, error) {
	cl.firstErrMu.Lock()
	defer cl.firstErrMu.Unlock()

	return cl.failures.Load(), cl.firstErr
}

// Get retrieves the resource from the API and unmarshals the JSON retrieved
func (cl *APIClient) Get(ctx context.Context, path string, obj any) error {
	path = strings.Replace(path, "/redfish/v1/", "", 1)

	b, err := cl.get(ctx, path)
	if err == nil {
		err = json.Unmarshal(b, obj)
		if err != nil {
			err = fmt.Errorf("could not parse response of %s: %w", path, err)
		}
	}

	if err != nil {
		cl.recordFailure(err)
	}

	return err
}

func (cl *APIClient) recordFailure(err error) {
	cl.failures.Add(1)

	cl.firstErrMu.Lock()
	defer cl.firstErrMu.Unlock()

	if cl.firstErr == nil {
		cl.firstErr = err
	}
}

func (cl *APIClient) get(ctx context.Context, path string) ([]byte, error) {
	baseURL := strings.TrimSuffix(cl.url, "/")
	relativePath := strings.TrimPrefix(path, "/")

	uri := baseURL
	if relativePath != "" {
		uri += "/" + relativePath
	}

	ctx, span := cl.tracer.Start(ctx, "Client.Get", trace.WithAttributes(
		attribute.String("URI", uri),
	))
	defer span.End()

	b, err := cl.do(ctx, uri)
	if err != nil {
		cl.logger.Debug("Request failed", "uri", uri, "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetStatus(codes.Ok, "")
	return b, nil
}

func (cl *APIClient) do(ctx context.Context, uri string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		// do not queue requests for scrapes which were canceled or timed out
		return nil, fmt.Errorf("GET %s: %w", uri, context.Cause(ctx))
	}

	cl.logger.Debug("GET", "uri", uri)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(cl.username, cl.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("OData-Version", "4.0")

	cl.requests.Add(1)
	resp, err := cl.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		// the connection can only be reused if the body was read completely
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: uri}
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, err
	}

	if len(b) > maxResponseSize {
		return nil, fmt.Errorf("response of %s exceeds %d bytes", uri, maxResponseSize)
	}

	if cl.logBodies {
		cl.logger.Debug("Response", "uri", uri, "status", resp.Status, "body", string(b))
	}

	return b, nil
}
