// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer = otel.GetTracerProvider().Tracer(
		"github.com/czerwonk/ilo_exporter",
		trace.WithSchemaURL(semconv.SchemaURL),
	)
)

func initTracing(ctx context.Context) (func(), error) {
	if !*tracingEnabled {
		return initTracingWithNoop()
	}

	switch *tracingProvider {
	case "stdout":
		return initTracingToStdOut(ctx)
	case "collector":
		return initTracingToCollector(ctx)
	default:
		log.Warnf("got invalid value for tracing.provider: %s, disable tracing", *tracingProvider)
		return initTracingWithNoop()
	}
}

func initTracingWithNoop() (func(), error) {
	tp := trace.NewNoopTracerProvider()
	otel.SetTracerProvider(tp)

	return func() {}, nil
}

func initTracingToStdOut(ctx context.Context) (func(), error) {
	log.Info("Initialize tracing (STDOUT)")

	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resourceDefinition()),
	)
	otel.SetTracerProvider(tp)

	return shutdownTraceProvider(ctx, tp.Shutdown), nil
}

func initTracingToCollector(ctx context.Context) (func(), error) {
	log.Infof("Initialize tracing (agent: %s)", *tracingCollectorEndpoint)

	cl := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(*tracingCollectorEndpoint),
	)
	exp, err := otlptrace.New(ctx, cl)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC collector exporter: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exp)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resourceDefinition()),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return shutdownTraceProvider(ctx, tp.Shutdown), nil
}

func shutdownTraceProvider(ctx context.Context, shutdownFunc func(ctx context.Context) error) func() {
	return func() {
		if err := shutdownFunc(ctx); err != nil {
			log.Errorf("failed to shutdown TracerProvider: %v", err)
		}
	}
}

func resourceDefinition() *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("ilo_exporter"),
		semconv.ServiceVersionKey.String(version),
	)
}
