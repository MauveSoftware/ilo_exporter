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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// maxResponseSize limits the size of a response body read from the API
	maxResponseSize = 32 << 20

	// maxNotReadyRetries limits the retries of requests for resources which are not ready yet
	maxNotReadyRetries = 2

	// maxNotReadyDelay limits the time to wait before retrying a request for a resource which is not ready yet
	maxNotReadyDelay = 10 * time.Second

	// defaultNotReadyDelay is used if the service does not specify when to retry
	defaultNotReadyDelay = 5 * time.Second
)

// HTTPError is returned when the API responds with a non-success status code
type HTTPError struct {
	StatusCode int
	URL        string

	// MessageID is the Redfish message ID of the error response (if any), e.g. "Base.1.4.InternalError"
	MessageID string

	// MessageArgs are the arguments of the message
	MessageArgs []string

	// NotReady is true if the resource is not ready yet and the request should be retried after RetryAfter
	NotReady   bool
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("%s returned %d %s", e.URL, e.StatusCode, http.StatusText(e.StatusCode))
	if e.MessageID != "" {
		msg += " (" + e.MessageID
		if len(e.MessageArgs) > 0 {
			msg += ": " + strings.Join(e.MessageArgs, ", ")
		}
		msg += ")"
	}

	return msg
}

// newHTTPError creates an error for a response with a non-success status code, including the
// Redfish message ID of the error response body, which explains why the request was rejected
func newHTTPError(resp *http.Response, uri string) *HTTPError {
	e := &HTTPError{StatusCode: resp.StatusCode, URL: uri}

	var body struct {
		Error struct {
			Code         string `json:"code"`
			ExtendedInfo []struct {
				MessageID   string   `json:"MessageId"`
				MessageArgs []string `json:"MessageArgs"`
			} `json:"@Message.ExtendedInfo"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return e
	}

	if info := body.Error.ExtendedInfo; len(info) > 0 && info[0].MessageID != "" {
		e.MessageID = info[0].MessageID
		e.MessageArgs = info[0].MessageArgs
	} else {
		e.MessageID = body.Error.Code
	}

	// iLO answers requests for resources it has to retrieve from devices first (e.g. storage of NVMe drives
	// managed by iLO 5) with 400 and iLO.2.x.ResourceNotReadyRetry, the first argument being the seconds to wait
	if strings.HasSuffix(e.MessageID, ".ResourceNotReadyRetry") {
		e.NotReady = true
		e.RetryAfter = defaultNotReadyDelay
		if len(e.MessageArgs) > 0 {
			if s, err := strconv.Atoi(strings.TrimSpace(strings.Split(e.MessageArgs[0], ",")[0])); err == nil && s >= 0 {
				e.RetryAfter = time.Duration(s) * time.Second
			}
		}
	}

	return e
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
	auth       Authenticator
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

// WithAuthenticator sets the authenticator (default: basic authentication)
func WithAuthenticator(a Authenticator) ClientOption {
	return func(cl *APIClient) {
		cl.auth = a
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
		auth:     &BasicAuth{Username: username, Password: password},
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

	uri := baseURL + "/" + relativePath

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
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			// do not queue requests for scrapes which were canceled or timed out
			return nil, fmt.Errorf("GET %s: %w", uri, context.Cause(ctx))
		}

		b, err := cl.fetch(ctx, uri)

		var he *HTTPError
		if attempt < maxNotReadyRetries && errors.As(err, &he) && he.NotReady {
			delay := min(he.RetryAfter, maxNotReadyDelay)
			cl.logger.Debug("Resource not ready", "uri", uri, "retry_after", delay)

			if wait(ctx, delay) {
				continue
			}
		}

		return b, err
	}
}

// wait waits for d, unless ctx is done earlier or its deadline does not leave time for a request after waiting
func wait(ctx context.Context, d time.Duration) bool {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < d+time.Second {
		return false
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (cl *APIClient) fetch(ctx context.Context, uri string) ([]byte, error) {
	resp, err := cl.send(ctx, uri)
	if err != nil {
		return nil, err
	}
	// the connection can only be reused if the body was read completely
	defer drainAndClose(resp)

	if resp.StatusCode >= 300 {
		return nil, newHTTPError(resp, uri)
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

// send sends a GET request. A request rejected with 401 is retried once if the authenticator renewed its credentials.
func (cl *APIClient) send(ctx context.Context, uri string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		cl.logger.Debug("GET", "uri", uri)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return nil, err
		}

		if err := cl.auth.Apply(ctx, req); err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("OData-Version", "4.0")

		cl.requests.Add(1)
		resp, err := cl.client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 && cl.auth.Unauthorized(req) {
			drainAndClose(resp)
			continue
		}

		return resp, nil
	}
}
