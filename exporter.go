// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/MauveSoftware/ilo_exporter/pkg/chassis"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/MauveSoftware/ilo_exporter/pkg/manager"
	"github.com/MauveSoftware/ilo_exporter/pkg/system"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// exporter handles scrape requests
type exporter struct {
	username   string
	password   string
	httpClient *http.Client
	targets    *targetFilter
	logger     *slog.Logger
	logBodies  bool

	// maxTimeout is the maximum duration of a scrape (0: unlimited)
	maxTimeout time.Duration
	// timeoutOffset is subtracted from the scrape timeout sent by Prometheus
	timeoutOffset time.Duration
}

// httpError is an error with a HTTP status code to respond with
type httpError struct {
	code int
	err  error
}

func (e *httpError) Error() string {
	return e.err.Error()
}

// ServeHTTP implements http.Handler
func (e *exporter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := e.handleMetricsRequest(w, r)
	if err == nil {
		return
	}

	code := http.StatusInternalServerError
	var he *httpError
	if errors.As(err, &he) {
		code = he.code
	}

	if code < http.StatusInternalServerError {
		e.logger.Debug("Rejected request", "remote", r.RemoteAddr, "err", err)
	} else {
		e.logger.Error("Request failed", "remote", r.RemoteAddr, "err", err)
	}

	http.Error(w, err.Error(), code)
}

func (e *exporter) handleMetricsRequest(w http.ResponseWriter, r *http.Request) error {
	host, err := parseTarget(r.URL.Query().Get("host"))
	if err != nil {
		return &httpError{code: http.StatusBadRequest, err: err}
	}

	if !e.targets.allows(host) {
		return &httpError{code: http.StatusForbidden, err: fmt.Errorf("host %s is not allowed", host)}
	}

	ctx, cancel := scrapeContext(r, e.maxTimeout, e.timeoutOffset)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HandleMetricsRequest", trace.WithAttributes(
		attribute.String("host", host),
	))
	defer span.End()

	opts := []client.ClientOption{
		client.WithHTTPClient(e.httpClient),
		client.WithLogger(e.logger.With("host", host)),
	}
	if e.logBodies {
		opts = append(opts, client.WithResponseLogging())
	}
	cl := client.NewClient(host, e.username, e.password, tracer, opts...)

	reg := prometheus.NewRegistry()
	reg.MustRegister(system.NewCollector(ctx, cl, tracer))
	reg.MustRegister(manager.NewCollector(ctx, cl, tracer))
	reg.MustRegister(chassis.NewCollector(ctx, cl, tracer))

	promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorLog:      slog.NewLogLogger(e.logger.Handler(), slog.LevelWarn),
		ErrorHandling: promhttp.ContinueOnError,
	}).ServeHTTP(w, r)

	if n, err := cl.Failures(); n > 0 {
		e.logger.Warn("Scrape incomplete", "host", host, "failed_requests", n, "err", err)
	}

	return nil
}

// scrapeContext limits the duration of a scrape to the scrape timeout sent by Prometheus (minus offset) and maxTimeout
func scrapeContext(r *http.Request, maxTimeout, offset time.Duration) (context.Context, context.CancelFunc) {
	timeout := maxTimeout

	if v := r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds"); v != "" {
		if s, err := strconv.ParseFloat(v, 64); err == nil && s > 0 {
			t := time.Duration(s*float64(time.Second)) - offset
			if t <= 0 {
				t = time.Duration(s * float64(time.Second))
			}

			if timeout <= 0 || t < timeout {
				timeout = t
			}
		}
	}

	if timeout <= 0 {
		return context.WithCancel(r.Context())
	}

	return context.WithTimeout(r.Context(), timeout)
}
