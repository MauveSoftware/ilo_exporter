// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"
)

// maxResponseSize limits the size of a response body read from the API
const maxResponseSize = 32 << 20

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
	sem        *semaphore.Weighted
	failures   atomic.Int64
	firstErrMu sync.Mutex
	firstErr   error
}

// ClientOption applies options to APIClient
type ClientOption func(*APIClient)

// WithInsecure disables TLS certificate validation
func WithInsecure() ClientOption {
	return func(c *APIClient) {
		tr := &http.Transport{
			// iLO controllers typically use self-signed certificates, so skipping verification
			// is an explicit opt-in via this option rather than the default client behavior.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- intentional opt-in for iLO self-signed certs
		}
		c.client = &http.Client{Transport: tr}
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

// WithMaxConcurrentRequests defines the maximum number of GET requests sent against API concurrently
func WithMaxConcurrentRequests(max uint) ClientOption {
	n := int64(math.MaxInt64)
	if uint64(max) <= math.MaxInt64 {
		n = int64(max)
	}

	return func(c *APIClient) {
		c.sem = semaphore.NewWeighted(n)
	}
}

// NewClient creates a new client instance
func NewClient(hostName, username, password string, tracer trace.Tracer, opts ...ClientOption) *APIClient {
	cl := &APIClient{
		url:      fmt.Sprintf("https://%s/redfish/v1/", hostName),
		hostName: hostName,
		username: username,
		password: password,
		client:   &http.Client{},
		tracer:   tracer,
		logger:   slog.New(slog.DiscardHandler),
		sem:      semaphore.NewWeighted(1),
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

// Failures returns the number of failed requests and the first error which occurred
func (cl *APIClient) Failures() (int64, error) {
	cl.firstErrMu.Lock()
	defer cl.firstErrMu.Unlock()

	return cl.failures.Load(), cl.firstErr
}

// Get retrieves the ressource from the API and unmashals the json retrieved
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
	if err := cl.sem.Acquire(context.Background(), 1); err != nil {
		return nil, err
	}
	defer cl.sem.Release(1)

	baseURL := strings.TrimSuffix(cl.url, "/")
	relativePath := strings.TrimPrefix(path, "/")

	uri := baseURL
	if relativePath != "" {
		uri += "/" + relativePath
	}

	_, span := cl.tracer.Start(ctx, "Client.Get", trace.WithAttributes(
		attribute.String("URI", uri),
	))
	defer span.End()

	b, err := cl.do(uri)
	if err != nil {
		cl.logger.Debug("Request failed", "uri", uri, "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetStatus(codes.Ok, "")
	return b, nil
}

func (cl *APIClient) do(uri string) ([]byte, error) {
	cl.logger.Debug("GET", "uri", uri)

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(cl.username, cl.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("OData-Version", "4.0")

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
		return nil, fmt.Errorf("%s returned %s", uri, resp.Status)
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
