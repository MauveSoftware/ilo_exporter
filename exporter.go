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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MauveSoftware/ilo_exporter/internal/lru"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/MauveSoftware/ilo_exporter/pkg/collector"
	"github.com/MauveSoftware/ilo_exporter/pkg/topology"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// buildInfo reports the version of the exporter with every scrape, so the deployed versions can be queried
var buildInfo = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
	Name:        "ilo_exporter_build_info",
	Help:        "Version of the exporter (always 1)",
	ConstLabels: prometheus.Labels{"version": version, "goversion": runtime.Version()},
}, func() float64 { return 1 })

// exporter handles scrape requests
type exporter struct {
	username   string
	password   string
	httpClient *http.Client
	targets    *targetFilter
	logger     *slog.Logger
	logBodies  bool
	noExpand   bool

	// topologies caches the discovered topology per host (nil: discover on every scrape)
	topologies *topology.Cache
	// states contains the state of the last scrape per host, so only changes are logged
	states   *lru.Cache[hostState]
	statesMu sync.Mutex

	// sessions contains the session per host if session authentication is used (nil: basic authentication)
	sessions   *lru.Cache[*client.SessionAuth]
	sessionsMu sync.Mutex
	logouts    sync.WaitGroup

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

	var collectorOpts []collector.Option
	if names := r.URL.Query()["collect[]"]; len(names) > 0 {
		for _, n := range names {
			if !slices.Contains(collector.Names(), n) {
				return &httpError{code: http.StatusBadRequest, err: fmt.Errorf("unknown collector %q (valid: %s)", n, strings.Join(collector.Names(), ", "))}
			}
		}
		collectorOpts = append(collectorOpts, collector.WithCollectors(names...))
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
	if e.sessions != nil {
		opts = append(opts, client.WithAuthenticator(e.session(host)))
	}
	if e.logBodies {
		opts = append(opts, client.WithResponseLogging())
	}
	cl := client.NewClient(host, e.username, e.password, tracer, opts...)

	collectorOpts = append(collectorOpts, collector.WithTopology(func(ctx context.Context) (*topology.Topology, error) {
		discover := func(ctx context.Context) (*topology.Topology, error) {
			return topology.Discover(ctx, cl)
		}

		if e.topologies == nil {
			return discover(ctx)
		}

		return e.topologies.Get(ctx, host, discover)
	}))

	if e.noExpand {
		collectorOpts = append(collectorOpts, collector.WithoutExpand())
	}

	col := collector.New(ctx, cl, tracer, collectorOpts...)
	reg := prometheus.NewRegistry()
	reg.MustRegister(col, buildInfo)

	// metrics which can not be gathered (e.g. duplicates) are dropped and logged by observe on state change only
	g := &recordingGatherer{Gatherer: reg}
	promhttp.HandlerFor(g, promhttp.HandlerOpts{
		ErrorLog:      slog.NewLogLogger(e.logger.Handler(), slog.LevelDebug),
		ErrorHandling: promhttp.ContinueOnError,
	}).ServeHTTP(w, r)

	e.observe(host, col.Result(), cl.Requests(), g.err)
	return nil
}

// recordingGatherer records the error of the last Gather call
type recordingGatherer struct {
	prometheus.Gatherer
	err error
}

// Gather implements prometheus.Gatherer
func (g *recordingGatherer) Gather() ([]*dto.MetricFamily, error) {
	mfs, err := g.Gatherer.Gather()
	g.err = err
	return mfs, err
}

// countErrors returns the number of errors of a gather error
func countErrors(err error) int {
	if err == nil {
		return 0
	}

	var me prometheus.MultiError
	if errors.As(err, &me) {
		return len(me)
	}

	return 1
}

// enableSessions enables session authentication, keeping sessions for at most maxHosts hosts
func (e *exporter) enableSessions(maxHosts int) {
	e.sessions = lru.New(maxHosts, lru.WithEvictCallback(func(_ string, a *client.SessionAuth) {
		// called with the cache locked, so the session is deleted asynchronously
		e.logouts.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := a.Close(ctx); err != nil {
				e.logger.Debug("Could not delete session", "err", err)
			}
		})
	}))
}

// session returns the session authenticator of a host
func (e *exporter) session(host string) *client.SessionAuth {
	key := topology.Key(host)

	e.sessionsMu.Lock()
	defer e.sessionsMu.Unlock()

	if a, found := e.sessions.Get(key); found {
		return a
	}

	a := client.NewSessionAuth(host, e.username, e.password, e.httpClient, e.logger.With("host", host))
	e.sessions.Set(key, a)
	return a
}

// close releases resources held on the iLOs (sessions)
func (e *exporter) close(ctx context.Context) {
	if e.sessions == nil {
		return
	}

	e.sessions.Purge()

	done := make(chan struct{})
	go func() {
		e.logouts.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		e.logger.Warn("Could not delete all sessions", "err", ctx.Err())
	}
}

// hostState is the state of the last scrape of a host
type hostState struct {
	up       bool
	failed   string
	firmware string
	uuid     string
	// dropped is the number of metrics which could not be gathered
	dropped int
}

// observe logs changes of the state of a host and invalidates its topology if it changed
func (e *exporter) observe(host string, res collector.Result, requests int64, gatherErr error) {
	cur := hostState{
		up:       res.Up,
		failed:   strings.Join(res.Failed, ","),
		firmware: res.FirmwareVersion,
		uuid:     res.SystemUUID,
		dropped:  countErrors(gatherErr),
	}

	key := topology.Key(host)
	e.statesMu.Lock()
	prev, known := e.states.Get(key)
	if cur.firmware == "" {
		cur.firmware = prev.firmware
	}
	if cur.uuid == "" {
		cur.uuid = prev.uuid
	}
	e.states.Set(key, cur)
	e.statesMu.Unlock()

	e.logger.Debug("Scrape finished", "host", host, "up", res.Up, "failed", cur.failed, "requests", requests, "err", res.Err)

	invalidate := res.StaleTopology
	if known && prev.firmware != "" && prev.firmware != cur.firmware {
		e.logger.Info("Firmware changed", "host", host, "from", prev.firmware, "to", cur.firmware)
		invalidate = true
	}
	if known && prev.uuid != "" && prev.uuid != cur.uuid {
		e.logger.Info("System changed", "host", host, "from", prev.uuid, "to", cur.uuid)
		invalidate = true
	}
	if invalidate && e.topologies != nil {
		e.topologies.Invalidate(host)
	}

	if cur.dropped > 0 && (!known || prev.dropped == 0) {
		var me prometheus.MultiError
		first := gatherErr
		if errors.As(gatherErr, &me) {
			first = me[0]
		}
		e.logger.Warn("Invalid metrics dropped", "host", host, "count", cur.dropped, "err", first)
	} else if cur.dropped == 0 && known && prev.dropped > 0 {
		e.logger.Info("Invalid metrics resolved", "host", host)
	}

	switch {
	case !cur.up && (!known || prev.up):
		e.logger.Warn("iLO down", "host", host, "err", res.Err)
		return
	case !cur.up:
		return
	case known && !prev.up:
		e.logger.Info("iLO up", "host", host)
	}

	if cur.failed != "" && (!known || !prev.up || prev.failed != cur.failed) {
		e.logger.Warn("Scrape incomplete", "host", host, "failed_collectors", cur.failed, "err", res.Err)
	} else if cur.failed == "" && known && prev.up && prev.failed != "" {
		e.logger.Info("Scrape complete", "host", host)
	}
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
