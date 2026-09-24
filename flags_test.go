// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"io"
	"testing"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name              string
		env               map[string]string
		args              []string
		wantMaxConcurrent uint
		wantTLS           bool
		wantErr           bool
	}{
		{
			name:              "defaults without env",
			wantMaxConcurrent: 4,
		},
		{
			name:              "API_MAX_CONCURRENT overrides default",
			env:               map[string]string{"API_MAX_CONCURRENT": "8"},
			wantMaxConcurrent: 8,
		},
		{
			name:              "CLI arg overrides API_MAX_CONCURRENT",
			env:               map[string]string{"API_MAX_CONCURRENT": "8"},
			args:              []string{"-api.max-concurrent-requests=2"},
			wantMaxConcurrent: 2,
		},
		{
			name:    "invalid API_MAX_CONCURRENT",
			env:     map[string]string{"API_MAX_CONCURRENT": "abc"},
			wantErr: true,
		},
		{
			name:              "CMD_FLAGS are applied",
			env:               map[string]string{"CMD_FLAGS": "-tls.enabled  -api.max-concurrent-requests=6"},
			wantMaxConcurrent: 6,
			wantTLS:           true,
		},
		{
			name:              "CLI arg overrides CMD_FLAGS",
			env:               map[string]string{"CMD_FLAGS": "-api.max-concurrent-requests=6"},
			args:              []string{"-api.max-concurrent-requests=3"},
			wantMaxConcurrent: 3,
		},
		{
			name:              "CMD_FLAGS overrides API_MAX_CONCURRENT",
			env:               map[string]string{"API_MAX_CONCURRENT": "8", "CMD_FLAGS": "-api.max-concurrent-requests=6"},
			wantMaxConcurrent: 6,
		},
		{
			name:    "unknown flag in CMD_FLAGS",
			env:     map[string]string{"CMD_FLAGS": "-does.not.exist"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			maxConcurrent := fs.Uint("api.max-concurrent-requests", 4, "")
			tls := fs.Bool("tls.enabled", false, "")
			getenv := func(key string) string { return tt.env[key] }

			err := parseFlags(fs, tt.args, getenv)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *maxConcurrent != tt.wantMaxConcurrent {
				t.Errorf("api.max-concurrent-requests: got %d, want %d", *maxConcurrent, tt.wantMaxConcurrent)
			}
			if *tls != tt.wantTLS {
				t.Errorf("tls.enabled: got %t, want %t", *tls, tt.wantTLS)
			}
		})
	}
}
