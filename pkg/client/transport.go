// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// TransportConfig configures the HTTP transport used for Redfish requests
type TransportConfig struct {
	// MaxConnsPerHost limits the number of connections (and so concurrent requests) per iLO.
	// Since the transport is shared, the limit applies across all concurrent scrapes. 0 means no limit.
	MaxConnsPerHost int

	// IdleConnTimeout is the time after which idle connections are closed
	IdleConnTimeout time.Duration

	// InsecureSkipVerify disables verification of the iLO certificates
	InsecureSkipVerify bool

	// CAFile is a PEM file with CA certificates to verify iLO certificates with (optional)
	CAFile string
}

// NewTransport creates a transport to be shared by all clients, so connections can be reused
// across scrapes and the per host connection limit is enforced across all scrapes.
func NewTransport(cfg TransportConfig) (*http.Transport, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// iLO controllers typically use self-signed certificates, so skipping verification is configurable
		InsecureSkipVerify: cfg.InsecureSkipVerify, // #nosec G402 -- explicit opt-in (-api.insecure-skip-verify)
	}

	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("could not read CA file: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in CA file %s", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	dialer := &net.Dialer{
		// dials are not canceled with the request, so a short timeout prevents unreachable
		// hosts from blocking connection slots
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	idle := cfg.MaxConnsPerHost
	if idle == 0 {
		idle = 16
	}

	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSClientConfig:       tlsCfg,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: time.Minute,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		MaxIdleConnsPerHost:   idle,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		// HTTP/1.1 only: the connection limit is only a limit of concurrent requests without multiplexing
		ForceAttemptHTTP2: false,
		TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
	}, nil
}
