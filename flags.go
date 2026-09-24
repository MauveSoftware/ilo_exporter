// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"strings"
)

// parseFlags parses args into fs, honoring the environment variables of the container image.
// Precedence (lowest to highest): API_MAX_CONCURRENT, CMD_FLAGS, args.
func parseFlags(fs *flag.FlagSet, args []string, getenv func(string) string) error {
	if v := getenv("API_MAX_CONCURRENT"); v != "" {
		if err := fs.Set("api.max-concurrent-requests", v); err != nil {
			return fmt.Errorf("invalid API_MAX_CONCURRENT: %w", err)
		}
	}

	// Splitting on whitespace matches the former shell entrypoint, which expanded $CMD_FLAGS unquoted
	envArgs := strings.Fields(getenv("CMD_FLAGS"))

	return fs.Parse(append(envArgs, args...))
}
