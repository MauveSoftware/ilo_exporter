// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

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

const version string = "1.0.3"

var (
	showVersion              = flag.Bool("version", false, "Print version information.")
	listenAddress            = flag.String("web.listen-address", ":9545", "Address on which to expose metrics and web interface.")
	metricsPath              = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	username                 = flag.String("api.username", "", "Username")
	password                 = flag.String("api.password", "", "Password")
	maxConcurrentRequests    = flag.Uint("api.max-concurrent-requests", 4, "Maximum number of requests sent against API concurrently")
	tlsEnabled               = flag.Bool("tls.enabled", false, "Enables TLS")
	tlsCertChainPath         = flag.String("tls.cert-file", "", "Path to TLS cert file")
	tlsKeyPath               = flag.String("tls.key-file", "", "Path to TLS key file")
	tracingEnabled           = flag.Bool("tracing.enabled", false, "Enables tracing using OpenTelemetry")
	tracingProvider          = flag.String("tracing.provider", "", "Sets the tracing provider (stdout or collector)")
	tracingCollectorEndpoint = flag.String("tracing.collector.grpc-endpoint", "", "Sets the tracing provider (stdout or collector)")
)

func init() {
	flag.Usage = func() {
		fmt.Println("Usage: ilo_exporter [ ... ]\n\nParameters:")
		fmt.Println()
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	shutdownTracing, err := initTracing(ctx)
	if err != nil {
		log.Fatalf("could not initialize tracing: %v", err)
	}
	defer shutdownTracing()

	startServer()
}

func printVersion() {
	fmt.Println("ilo_exporter")
	fmt.Printf("Version: %s\n", version)
	fmt.Println("Author(s): Daniel Czerwonk")
	fmt.Println("Copyright: 2022, Mauve Mailorder Software GmbH & Co. KG, Licensed under MIT license")
	fmt.Println("Metric exporter for HP iLO")
}

func startServer() {
	logrus.Infof("Starting iLO exporter (Version: %s)", version)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>iLO5 Exporter (Version ` + version + `)</title></head>
			<body>
			<h1>iLO Exporter by Mauve Mailorder Software</h1>
			<h2>Example</h2>
			<p>Metrics for host 172.16.0.200</p>
			<p><a href="` + *metricsPath + `?host=172.16.0.200">` + r.Host + *metricsPath + `?host=172.16.0.200</a></p>
			<h2>More information</h2>
			<p><a href="https://github.com/MauveSoftware/ilo_exporter">github.com/MauveSoftware/ilo_exporter</a></p>
			</body>
			</html>`))
	})
	http.HandleFunc(*metricsPath, errorHandler(handleMetricsRequest))

	logrus.Infof("Listening for %s on %s (TLS: %v)", *metricsPath, *listenAddress, *tlsEnabled)
	if *tlsEnabled {
		logrus.Fatal(http.ListenAndServeTLS(*listenAddress, *tlsCertChainPath, *tlsKeyPath, nil))
		return
	}

	logrus.Fatal(http.ListenAndServe(*listenAddress, nil))
}

func errorHandler(f func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := f(w, r)

		if err != nil {
			logrus.Errorln(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleMetricsRequest(w http.ResponseWriter, r *http.Request) error {
	host := r.URL.Query().Get("host")

	ctx, span := tracer.Start(r.Context(), "HandleMetricsRequest", trace.WithAttributes(
		attribute.String("host", host),
	))
	defer span.End()

	if host == "" {
		return fmt.Errorf("no host defined")
	}

	reg := prometheus.NewRegistry()

	cl := client.NewClient(host, *username, *password, tracer, client.WithMaxConcurrentRequests(*maxConcurrentRequests), client.WithInsecure(), client.WithDebug())
	reg.MustRegister(system.NewCollector(ctx, cl, tracer))
	reg.MustRegister(manager.NewCollector(ctx, cl, tracer))
	reg.MustRegister(chassis.NewCollector(ctx, cl, tracer))

	l := logrus.New()
	l.Level = logrus.ErrorLevel

	promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorLog:      l,
		ErrorHandling: promhttp.ContinueOnError}).ServeHTTP(w, r)
	return nil
}
