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
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MauveSoftware/ilo_exporter/pkg/chassis"
	"github.com/MauveSoftware/ilo_exporter/pkg/client"
	"github.com/MauveSoftware/ilo_exporter/pkg/manager"
	"github.com/MauveSoftware/ilo_exporter/pkg/system"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// version is set at build time using -ldflags "-X main.version=..."
var version = "1.1.3"

var (
	showVersion              = flag.Bool("version", false, "Print version information.")
	listenAddress            = flag.String("web.listen-address", ":9545", "Address on which to expose metrics and web interface.")
	metricsPath              = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	writeTimeout             = flag.Duration("web.write-timeout", 2*time.Minute, "Maximum duration for writing a response incl. collecting metrics from the API (0 disables the timeout)")
	username                 = flag.String("api.username", "", "Username")
	password                 = flag.String("api.password", "", "Password")
	maxConcurrentRequests    = flag.Uint("api.max-concurrent-requests", 4, "Maximum number of requests sent against API concurrently")
	tlsEnabled               = flag.Bool("tls.enabled", false, "Enables TLS")
	tlsCertChainPath         = flag.String("tls.cert-file", "", "Path to TLS cert file")
	tlsKeyPath               = flag.String("tls.key-file", "", "Path to TLS key file")
	tracingEnabled           = flag.Bool("tracing.enabled", false, "Enables tracing using OpenTelemetry")
	tracingProvider          = flag.String("tracing.provider", "", "Sets the tracing provider (stdout or collector)")
	tracingCollectorEndpoint = flag.String("tracing.collector.grpc-endpoint", "", "Sets the gRPC endpoint of the OpenTelemetry collector")
	allowedTargets           = flag.String("web.allowed-targets", "", "Comma separated list of host names, domain suffixes (.example.com), IP addresses and CIDR prefixes which may be scraped (empty: all hosts, the configured credentials are sent to every requested host)")
)

var targets *targetFilter

func init() {
	flag.Usage = func() {
		fmt.Println("Usage: ilo_exporter [ ... ]\n\nParameters:")
		fmt.Println()
		flag.PrintDefaults()
	}
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

	var err error
	targets, err = parseTargetFilter(*allowedTargets)
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdownTracing, err := initTracing(ctx)
	if err != nil {
		logrus.Errorf("could not initialize tracing: %v", err)
		return 1
	}
	defer shutdownTracing()

	if err := startServer(ctx); err != nil {
		logrus.Error(err)
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

func startServer(ctx context.Context) error {
	logrus.Infof("Starting iLO exporter (Version: %s)", version)
	logTargetFilter(logrus.StandardLogger(), targets, *allowedTargets)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
			logrus.Errorf("failed to write response: %v", err)
		}
	})
	http.HandleFunc(*metricsPath, errorHandler(handleMetricsRequest))

	logrus.Infof("Listening for %s on %s (TLS: %v)", *metricsPath, *listenAddress, *tlsEnabled)
	srv := &http.Server{
		Addr:              *listenAddress,
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

	logrus.Info("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return nil
}

// httpError is an error with a HTTP status code to respond with
type httpError struct {
	code int
	err  error
}

func (e *httpError) Error() string {
	return e.err.Error()
}

func errorHandler(f func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := f(w, r)
		if err == nil {
			return
		}

		code := http.StatusInternalServerError
		var he *httpError
		if errors.As(err, &he) {
			code = he.code
		}

		if code < http.StatusInternalServerError {
			logrus.Debugf("rejected request from %s: %v", r.RemoteAddr, err)
		} else {
			logrus.Errorln(err)
		}

		http.Error(w, err.Error(), code)
	}
}

func handleMetricsRequest(w http.ResponseWriter, r *http.Request) error {
	host, err := parseTarget(r.URL.Query().Get("host"))
	if err != nil {
		return &httpError{code: http.StatusBadRequest, err: err}
	}

	if !targets.allows(host) {
		return &httpError{code: http.StatusForbidden, err: fmt.Errorf("host %s is not allowed", host)}
	}

	ctx, span := tracer.Start(r.Context(), "HandleMetricsRequest", trace.WithAttributes(
		attribute.String("host", host),
	))
	defer span.End()

	reg := prometheus.NewRegistry()

	cl := client.NewClient(host, *username, *password, tracer, client.WithMaxConcurrentRequests(*maxConcurrentRequests), client.WithInsecure(), client.WithDebug())
	reg.MustRegister(system.NewCollector(ctx, cl, tracer))
	reg.MustRegister(manager.NewCollector(ctx, cl, tracer))
	reg.MustRegister(chassis.NewCollector(ctx, cl, tracer))

	l := logrus.New()
	l.Level = logrus.ErrorLevel

	promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorLog:      l,
		ErrorHandling: promhttp.ContinueOnError,
	}).ServeHTTP(w, r)
	return nil
}
