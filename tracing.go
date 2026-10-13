// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	tracer = otel.GetTracerProvider().Tracer(
		"github.com/MauveSoftware/ilo_exporter",
		trace.WithSchemaURL(semconv.SchemaURL),
	)
)

func initTracing(ctx context.Context, logger *slog.Logger) (func(), error) {
	if !*tracingEnabled {
		return initTracingWithNoop()
	}

	switch *tracingProvider {
	case "stdout":
		return initTracingToStdOut(logger)
	case "collector":
		return initTracingToCollector(ctx, logger)
	default:
		logger.Warn("Invalid value for tracing.provider, disabling tracing", "provider", *tracingProvider)
		return initTracingWithNoop()
	}
}

func initTracingWithNoop() (func(), error) {
	tp := noop.NewTracerProvider()
	otel.SetTracerProvider(tp)

	return func() {}, nil
}

func initTracingToStdOut(logger *slog.Logger) (func(), error) {
	logger.Info("Initialize tracing (STDOUT)")

	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resourceDefinition()),
	)
	otel.SetTracerProvider(tp)

	return shutdownTraceProvider(tp.Shutdown, logger), nil
}

func initTracingToCollector(ctx context.Context, logger *slog.Logger) (func(), error) {
	logger.Info("Initialize tracing", "collector", *tracingCollectorEndpoint)

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

	return shutdownTraceProvider(tp.Shutdown, logger), nil
}

func shutdownTraceProvider(shutdownFunc func(ctx context.Context) error, logger *slog.Logger) func() {
	return func() {
		// the context passed to initTracing is already canceled on shutdown, so use a new one to flush pending spans
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := shutdownFunc(ctx); err != nil {
			logger.Error("Failed to shutdown TracerProvider", "err", err)
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
