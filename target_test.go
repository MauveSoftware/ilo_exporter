// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "172.16.0.200", want: "172.16.0.200"},
		{raw: "ilo-01.example.com", want: "ilo-01.example.com"},
		{raw: "ilo-01.example.com:8443", want: "ilo-01.example.com:8443"},
		{raw: "::1", want: "[::1]"},
		{raw: "[2001:db8::1]:443", want: "[2001:db8::1]:443"},
		{raw: "", wantErr: true},
		{raw: "evil.example#", wantErr: true},
		{raw: "evil.example/foo", wantErr: true},
		{raw: "evil.example?x=1", wantErr: true},
		{raw: "user:pass@evil.example", wantErr: true},
		{raw: "ilo 01", wantErr: true},
		{raw: "ilo\n01", wantErr: true},
		{raw: "ilo:0", wantErr: true},
		{raw: "ilo:99999", wantErr: true},
		{raw: "ilo:abc", wantErr: true},
		{raw: "\xff", wantErr: true},
		{raw: "fe80::1%25eth0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseTarget(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTargetFilter(t *testing.T) {
	f, err := parseTargetFilter("10.0.0.0/8, ilo-01.Example.com,192.168.1.10,2001:db8::/32, .ILO.example.net, *.mgmt.example.org.")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]bool{
		"10.1.2.3":               true,
		"10.1.2.3:443":           true,
		"11.1.2.3":               false,
		"ilo-01.example.com":     true,
		"ILO-01.example.com:443": true,
		"ilo-01.example.com.":    true,
		"ilo-02.example.com":     false,
		"192.168.1.10":           true,
		"192.168.1.11":           false,
		"[2001:db8::1]":          true,
		"[2001:db9::1]:443":      false,

		// domain suffixes match all hosts below the domain, but not the domain itself
		"srv1.ilo.example.net":          true,
		"SRV1.dc2.ilo.example.net:8443": true,
		"srv1.ilo.example.net.":         true,
		"ilo.example.net":               false,
		"srv1ilo.example.net":           false,
		"srv1.ilo.example.net.evil.com": false,
		"srv1.mgmt.example.org":         true,
		"mgmt.example.org":              false,
	}

	for addr, want := range tests {
		if got := f.allows(addr); got != want {
			t.Errorf("allows(%q) = %v, want %v", addr, got, want)
		}
	}

	var none *targetFilter
	if !none.allows("anything") {
		t.Error("nil filter should allow all targets")
	}

	for _, invalid := range []string{"10.0.0.0/33", ".", "*.", "*", ".10.0.0.1", ".exa mple.com", ".example..com", "*.ex/ample.com", "ilo 01.example.com"} {
		if _, err := parseTargetFilter(invalid); err == nil {
			t.Errorf("expected error for %q", invalid)
		}
	}
}

func TestTargetFilterSuffixDoesNotMatchIP(t *testing.T) {
	f, err := parseTargetFilter(".1.example.com")
	if err != nil {
		t.Fatal(err)
	}

	if f.allows("10.0.0.1") {
		t.Error("domain suffix must not match IP addresses")
	}
}

func TestLogTargetFilter(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logTargetFilter(logger, nil, "")
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "every host") {
		t.Errorf("expected a warning without allowed targets, got %q", out)
	}

	buf.Reset()
	f, err := parseTargetFilter(".ilo.example.com")
	if err != nil {
		t.Fatal(err)
	}
	logTargetFilter(logger, f, ".ilo.example.com")
	if out := buf.String(); !strings.Contains(out, "level=INFO") || !strings.Contains(out, "allowed_targets=.ilo.example.com") {
		t.Errorf("expected the allowed targets to be logged, got %q", out)
	}
}
