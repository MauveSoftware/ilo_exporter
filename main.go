// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/prometheus/common/promslog"
)

// version is set at build time using -ldflags "-X main.version=..."
var version = "1.1.3"

var (
	showVersion              = flag.Bool("version", false, "Print version information.")
	listenAddress            = flag.String("web.listen-address", ":9545", "Address on which to expose metrics and web interface.")
	metricsPath              = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	writeTimeout             = flag.Duration("web.write-timeout", 2*time.Minute, "Maximum duration for writing a response incl. collecting metrics from the API (0 disables the timeout)")
	timeoutOffset            = flag.Duration("web.timeout-offset", 500*time.Millisecond, "Offset to subtract from the scrape timeout sent by Prometheus, leaving time to send the response")
	allowedTargets           = flag.String("web.allowed-targets", "", "Comma separated list of host names, domain suffixes (.example.com), IP addresses and CIDR prefixes which may be scraped (empty: all hosts, the configured credentials are sent to every requested host)")
	username                 = flag.String("api.username", "", "Username")
	password                 = flag.String("api.password", "", "Password")
	maxConcurrentRequests    = flag.Uint("api.max-concurrent-requests", 4, "Maximum number of requests sent to an iLO concurrently, across all scrapes of the iLO (0: unlimited)")
	apiTimeout               = flag.Duration("api.timeout", 0, "Maximum duration of a scrape (0: limited by the scrape timeout sent by Prometheus and -web.write-timeout)")
	idleConnTimeout          = flag.Duration("api.idle-conn-timeout", time.Minute, "Time after which idle connections to an iLO are closed")
	insecureSkipVerify       = flag.Bool("api.insecure-skip-verify", true, "Skip verification of iLO certificates (ignored if -api.ca-file is set)")
	caFile                   = flag.String("api.ca-file", "", "PEM file with CA certificates to verify iLO certificates with")
	tlsEnabled               = flag.Bool("tls.enabled", false, "Enables TLS")
	tlsCertChainPath         = flag.String("tls.cert-file", "", "Path to TLS cert file")
	tlsKeyPath               = flag.String("tls.key-file", "", "Path to TLS key file")
	tracingEnabled           = flag.Bool("tracing.enabled", false, "Enables tracing using OpenTelemetry")
	tracingProvider          = flag.String("tracing.provider", "", "Sets the tracing provider (stdout or collector)")
	tracingCollectorEndpoint = flag.String("tracing.collector.grpc-endpoint", "", "Sets the gRPC endpoint of the OpenTelemetry collector")
	logRedfishBodies         = flag.Bool("log.redfish-bodies", false, "Log the body of every Redfish response (requires -log.level=debug)")
	logLevel                 = promslog.NewLevel()
	logFormat                = promslog.NewFormat()
)

func init() {
	_ = logLevel.Set("info")
	_ = logFormat.Set("logfmt")
	flag.Var(levelFlag{logLevel}, "log.level", "Only log messages with the given severity or above. One of: [debug, info, warn, error]")
	flag.Var(logFormat, "log.format", "Output format of log messages. One of: [logfmt, json]")

	flag.Usage = func() {
		fmt.Println("Usage: ilo_exporter [ ... ]\n\nParameters:")
		fmt.Println()
		flag.PrintDefaults()
	}
}

// levelFlag wraps promslog.Level, since flag.PrintDefaults can not print its zero value
type levelFlag struct {
	*promslog.Level
}

func (l levelFlag) String() string {
	if l.Level == nil {
		return ""
	}

	return l.Level.String()
}

func main() {
	if err := parseFlags(flag.CommandLine, os.Args[1:], os.Getenv); err != nil {
		log.Fatal(err)
	}

	if *username == "" {
		*username = os.Getenv("API_USERNAME")
	}
	if *password == "" {
		*password = os.Getenv("API_PASSWORD")
	}

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	logger := promslog.New(&promslog.Config{Level: logLevel, Format: logFormat})

	e, err := newExporterFromFlags(logger)
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(run(e, logger))
}

func newExporterFromFlags(logger *slog.Logger) (*exporter, error) {
	targets, err := parseTargetFilter(*allowedTargets)
	if err != nil {
		return nil, err
	}

	maxConns := int(min(*maxConcurrentRequests, math.MaxInt32))
	tr, err := client.NewTransport(client.TransportConfig{
		MaxConnsPerHost:    maxConns,
		IdleConnTimeout:    *idleConnTimeout,
		InsecureSkipVerify: *insecureSkipVerify && *caFile == "",
		CAFile:             *caFile,
	})
	if err != nil {
		return nil, err
	}

	maxTimeout := *apiTimeout
	if *writeTimeout > 0 {
		// leave some time to write the response
		limit := *writeTimeout - *timeoutOffset
		if maxTimeout <= 0 || limit < maxTimeout {
			maxTimeout = limit
		}
	}

	return &exporter{
		username:      *username,
		password:      *password,
		httpClient:    &http.Client{Transport: tr},
		targets:       targets,
		logger:        logger,
		logBodies:     *logRedfishBodies,
		maxTimeout:    maxTimeout,
		timeoutOffset: *timeoutOffset,
	}, nil
}

func run(e *exporter, logger *slog.Logger) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdownTracing, err := initTracing(ctx, logger)
	if err != nil {
		logger.Error("Could not initialize tracing", "err", err)
		return 1
	}
	defer shutdownTracing()

	if err := startServer(ctx, e, logger); err != nil {
		logger.Error("Server failed", "err", err)
		return 1
	}

	return 0
}

func printVersion() {
	fmt.Println("ilo_exporter")
	fmt.Printf("Version: %s\n", version)
	fmt.Println("Author(s): Daniel Czerwonk")
	fmt.Println("Copyright: 2022, Mauve Mailorder Software GmbH & Co. KG, Licensed under MIT license")
	fmt.Println("Metric exporter for HP iLO")
}

func startServer(ctx context.Context, e *exporter, logger *slog.Logger) error {
	logger.Info("Starting iLO exporter", "version", version)
	logTargetFilter(logger, e.targets, *allowedTargets)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
			<head><title>iLO Exporter (Version ` + version + `)</title></head>
			<body>
			<h1>iLO Exporter by Mauve Mailorder Software</h1>
			<h2>Example</h2>
			<p>Metrics for host 172.16.0.200</p>
			<p><a href="` + *metricsPath + `?host=172.16.0.200">` + html.EscapeString(r.Host) + *metricsPath + `?host=172.16.0.200</a></p>
			<h2>More information</h2>
			<p><a href="https://github.com/MauveSoftware/ilo_exporter">github.com/MauveSoftware/ilo_exporter</a></p>
			</body>
			</html>`))
		if err != nil {
			logger.Debug("Failed to write response", "err", err)
		}
	})
	mux.Handle(*metricsPath, e)

	logger.Info("Listening", "address", *listenAddress, "path", *metricsPath, "tls", *tlsEnabled)
	srv := &http.Server{
		Addr:              *listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      *writeTimeout,
		// cancels running scrapes on shutdown
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		if *tlsEnabled {
			errCh <- srv.ListenAndServeTLS(*tlsCertChainPath, *tlsKeyPath)
			return
		}

		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return nil
}
