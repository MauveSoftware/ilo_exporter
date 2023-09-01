// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/MauveSoftware/ilo5_exporter/pkg/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func NewCollectorContext(ctx context.Context, cl client.Client, ch chan<- prometheus.Metric, tracer trace.Tracer) *CollectorContext {
	return &CollectorContext{
		rootCtx:  ctx,
		cl:       cl,
		ch:       ch,
		tracer:   tracer,
		wg:       &sync.WaitGroup{},
		errCount: 0,
	}
}

type CollectorContext struct {
	rootCtx  context.Context
	wg       *sync.WaitGroup
	ch       chan<- prometheus.Metric
	tracer   trace.Tracer
	cl       client.Client
	errCount int32
}

func (cc *CollectorContext) Client() client.Client {
	return cc.cl
}

func (cc *CollectorContext) RootCtx() context.Context {
	return cc.rootCtx
}

func (cc *CollectorContext) WaitGroup() *sync.WaitGroup {
	return cc.wg
}

func (cc *CollectorContext) Tracer() trace.Tracer {
	return cc.tracer
}

func (cc *CollectorContext) RecordMetrics(metrics ...prometheus.Metric) {
	for _, m := range metrics {
		cc.ch <- m
	}
}

func (cc *CollectorContext) ErrCount() int32 {
	return cc.errCount
}

func (cc *CollectorContext) HandleError(err error, span trace.Span) {
	atomic.AddInt32(&cc.errCount, 1)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	logrus.Error(err.Error())
}
