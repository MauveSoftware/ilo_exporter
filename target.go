// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// parseTarget validates the host parameter of a scrape request and returns it in a form
// usable as the host part of an URL. Anything but host[:port] is rejected, so a crafted
// parameter can not redirect the request (and the credentials) to another path or host.
func parseTarget(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("no host defined")
	}

	invalid := fmt.Errorf("invalid host %q", raw)
	if !utf8.ValidString(raw) || strings.ContainsAny(raw, "/?#@\\%") {
		return "", invalid
	}
	if strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", invalid
	}

	addr := raw
	if ip, err := netip.ParseAddr(raw); err == nil && ip.Is6() {
		addr = "[" + raw + "]"
	}

	u, err := url.Parse("https://" + addr)
	if err != nil || u.Host != addr || u.Hostname() == "" {
		return "", invalid
	}

	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", invalid
		}
	}

	return addr, nil
}

// targetFilter restricts the hosts which can be scraped
type targetFilter struct {
	hosts    map[string]bool
	suffixes []string
	prefixes []netip.Prefix
}

// parseTargetFilter parses a comma separated list of host names, domain suffixes (.example.com or
// *.example.com), IP addresses and CIDR prefixes. An empty list allows all targets.
func parseTargetFilter(s string) (*targetFilter, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	f := &targetFilter{hosts: make(map[string]bool)}
	for _, e := range strings.Split(s, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}

		if strings.HasPrefix(e, ".") || strings.HasPrefix(e, "*.") {
			domain := normalizeHost(strings.TrimPrefix(strings.TrimPrefix(e, "*"), "."))
			if !validDomain(domain) {
				return nil, fmt.Errorf("invalid allowed target %q: not a domain suffix", e)
			}
			f.suffixes = append(f.suffixes, "."+domain)
			continue
		}

		if strings.Contains(e, "/") {
			p, err := netip.ParsePrefix(e)
			if err != nil {
				return nil, fmt.Errorf("invalid allowed target %q: %w", e, err)
			}
			f.prefixes = append(f.prefixes, p.Masked())
			continue
		}

		if ip, err := netip.ParseAddr(e); err == nil {
			f.prefixes = append(f.prefixes, netip.PrefixFrom(ip.Unmap(), ip.Unmap().BitLen()))
			continue
		}

		host := normalizeHost(e)
		if !validDomain(host) {
			return nil, fmt.Errorf("invalid allowed target %q: not a host name, domain suffix, IP address or CIDR prefix", e)
		}
		f.hosts[host] = true
	}

	return f, nil
}

// allows checks if the target (as returned by parseTarget) may be scraped.
// Domain suffixes only match host names, IP prefixes only match targets given as IP address:
// host names are not resolved.
func (f *targetFilter) allows(addr string) bool {
	if f == nil {
		return true
	}

	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")

	ip, err := netip.ParseAddr(host)
	if err != nil {
		host = normalizeHost(host)
		if f.hosts[host] {
			return true
		}

		for _, s := range f.suffixes {
			if strings.HasSuffix(host, s) {
				return true
			}
		}

		return false
	}

	for _, p := range f.prefixes {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}

	return false
}

// logTargetFilter logs at startup which hosts may be scraped (raw is the value of -web.allowed-targets)
func logTargetFilter(logger *slog.Logger, f *targetFilter, raw string) {
	if f == nil {
		logger.Warn("No allowed targets configured (-web.allowed-targets): the configured credentials are sent to " +
			"every host requested via the host parameter")
		return
	}

	logger.Info("Scrape targets restricted", "allowed_targets", raw)
}

// normalizeHost returns the host name in lower case without the trailing dot of a fully qualified name
func normalizeHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(h), ".")
}

// validDomain checks if d consists of dot separated labels of letters, digits, hyphens and underscores
// and is not an IP address
func validDomain(d string) bool {
	if d == "" {
		return false
	}
	if _, err := netip.ParseAddr(d); err == nil {
		return false
	}

	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				return false
			}
		}
	}

	return true
}
