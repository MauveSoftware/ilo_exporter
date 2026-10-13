// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

// Package collector collects metrics of an iLO using its Redfish API
package collector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const prefix = "ilo_"

// allDescs contains all metric descriptions, see newDesc
var allDescs []*prometheus.Desc

// newDesc creates a metric description and registers it for Describe
func newDesc(name, help string, labels ...string) *prometheus.Desc {
	d := prometheus.NewDesc(name, help, labels, nil)
	allDescs = append(allDescs, d)
	return d
}

var (
	upDesc                = newDesc(prefix+"up", "1 if the system resource of the iLO could be retrieved, 0 otherwise", "host")
	collectorSuccessDesc  = newDesc(prefix+"scrape_collector_success", "1 if all resources of the collector could be retrieved, 0 otherwise", "host", "collector")
	collectorDurationDesc = newDesc(prefix+"scrape_collector_duration_seconds", "Duration of the collector in seconds", "host", "collector")
	requestsDesc          = newDesc(prefix+"scrape_redfish_requests", "Number of Redfish requests sent during the scrape", "host")
)

// errSkipped is the error of sub collectors which were not run since the iLO is unreachable
var errSkipped = errors.New("skipped")

// group is a set of sub collectors sharing a scrape duration metric (as the former system, chassis and manager collectors)
type group struct {
	durationDesc *prometheus.Desc
	// emitOnError emits the duration even if a sub collector failed
	emitOnError bool
}

var (
	systemGroup = &group{
		durationDesc: newDesc(prefix+"system_scrape_duration_second", "Scrape duration for the system module", "host"),
	}
	chassisGroup = &group{
		durationDesc: newDesc(prefix+"chassis_scrape_duration_second", "Scrape duration for the chassis module", "host"),
		emitOnError:  true,
	}
	managerGroup = &group{
		durationDesc: newDesc(prefix+"manager_scrape_duration_second", "Scrape duration for the manager module", "host"),
	}
	groups = []*group{systemGroup, chassisGroup, managerGroup}
)

// subCollector collects a subset of the metrics. Sub collectors run concurrently.
type subCollector struct {
	name  string
	group *group
	run   func(ctx context.Context, s *scrape) error
}

// subCollectors in order of start. Sub collectors with few requests are started first, so they
// do not have to wait for a connection while a collector requesting many resources is running.
var subCollectors = []subCollector{
	{name: "manager", group: managerGroup, run: collectManager},
	{name: "power", group: chassisGroup, run: collectPower},
	{name: "thermal", group: chassisGroup, run: collectThermal},
	{name: "processor", group: systemGroup, run: collectProcessors},
	{name: "storage", group: systemGroup, run: collectStorage},
	{name: "memory", group: systemGroup, run: collectMemory},
}

// Names returns the names of the collectors which can be enabled or disabled
func Names() []string {
	names := make([]string, len(subCollectors))
	for i, sc := range subCollectors {
		names[i] = sc.name
	}

	return names
}

// Collector collects all metrics of an iLO. A collector is intended to be used for a single scrape.
type Collector struct {
	ctx     context.Context
	cl      client.Client
	tracer  trace.Tracer
	enabled []subCollector
}

// Option configures a Collector
type Option func(*Collector)

// WithCollectors only enables the given collectors (see Names). The system resource is always collected.
func WithCollectors(names ...string) Option {
	return func(c *Collector) {
		c.enabled = nil
		for _, sc := range subCollectors {
			if slices.Contains(names, sc.name) {
				c.enabled = append(c.enabled, sc)
			}
		}
	}
}

// New returns a new collector. All requests are bound to ctx.
func New(ctx context.Context, cl client.Client, tracer trace.Tracer, opts ...Option) *Collector {
	c := &Collector{
		ctx:     ctx,
		cl:      cl,
		tracer:  tracer,
		enabled: subCollectors,
	}

	for _, o := range opts {
		o(c)
	}

	return c
}

// Describe implements prometheus.Collector interface
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range allDescs {
		ch <- d
	}
}

// Collect implements prometheus.Collector interface
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	ctx, span := c.tracer.Start(c.ctx, "Collector.Collect")
	defer span.End()

	s := &scrape{
		cl:          c.cl,
		tracer:      c.tracer,
		ch:          ch,
		host:        c.cl.HostName(),
		systemPath:  "Systems/1",
		chassisPath: "Chassis/1",
		managerPath: "Managers/1",
	}

	// the system resource is retrieved first: if it fails because the iLO is unreachable or the
	// credentials are invalid, the remaining requests are not sent
	sysErr := s.run(ctx, "system", collectSystem)
	s.emitResult("system", sysErr, time.Since(start))
	s.emit(upDesc, boolValue(sysErr == nil), s.host)

	type result struct {
		err      error
		duration time.Duration
		end      time.Time
	}
	results := make([]result, len(c.enabled))

	if client.IsFatal(sysErr) {
		for i := range results {
			results[i] = result{err: errSkipped, end: time.Now()}
		}
	} else {
		var wg sync.WaitGroup
		for i, sc := range c.enabled {
			wg.Go(func() {
				scStart := time.Now()
				err := s.run(ctx, sc.name, sc.run)
				results[i] = result{err: err, duration: time.Since(scStart), end: time.Now()}
			})
		}
		wg.Wait()
	}

	for i, sc := range c.enabled {
		if results[i].err == errSkipped {
			s.emit(collectorSuccessDesc, 0, s.host, sc.name)
			continue
		}

		s.emitResult(sc.name, results[i].err, results[i].duration)
	}

	// errors of the system resource do not affect the scrape duration metric of the system module
	for _, g := range groups {
		var end time.Time
		enabled, failed := false, false
		for i, sc := range c.enabled {
			if sc.group != g {
				continue
			}

			enabled = true
			failed = failed || results[i].err != nil
			if results[i].end.After(end) {
				end = results[i].end
			}
		}

		if !enabled || (failed && !g.emitOnError) {
			continue
		}

		s.emit(g.durationDesc, end.Sub(start).Seconds(), s.host)
	}

	s.emit(requestsDesc, float64(c.cl.Requests()), s.host)
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}

	return 0
}

// scrape is the state of a single collection shared by all sub collectors
type scrape struct {
	cl     client.Client
	tracer trace.Tracer
	ch     chan<- prometheus.Metric
	host   string

	systemPath  string
	chassisPath string
	managerPath string
}

// run runs a sub collector in a span and recovers from panics
func (s *scrape) run(ctx context.Context, name string, f func(ctx context.Context, s *scrape) error) (err error) {
	ctx, span := s.tracer.Start(ctx, "Collector."+name)
	defer span.End()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s collector panicked: %v", name, r)
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	return f(ctx, s)
}

// emitResult sends the success and duration metrics of a collector
func (s *scrape) emitResult(name string, err error, duration time.Duration) {
	s.emit(collectorSuccessDesc, boolValue(err == nil), s.host, name)
	s.emit(collectorDurationDesc, duration.Seconds(), s.host, name)
}

// emit sends a gauge metric
func (s *scrape) emit(desc *prometheus.Desc, value float64, labels ...string) {
	m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, labels...)
	if err != nil {
		m = prometheus.NewInvalidMetric(desc, err)
	}

	s.ch <- m
}

// forEach calls f concurrently for all items and returns all errors. The number of concurrent
// requests is limited by the HTTP transport.
func forEach[T any](ctx context.Context, items []T, f func(ctx context.Context, item T) error) error {
	errs := make([]error, len(items))

	var wg sync.WaitGroup
	for i, item := range items {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("panic: %v", r)
				}
			}()

			errs[i] = f(ctx, item)
		})
	}
	wg.Wait()

	return errors.Join(errs...)
}
