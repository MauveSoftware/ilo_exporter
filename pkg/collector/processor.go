// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/MauveSoftware/ilo_exporter/pkg/redfish"
)

const processorPrefix = prefix + "processor_"

var (
	processorCountDesc   = newDesc(processorPrefix+"count", "Number of processors", "host")
	processorCoresDesc   = newDesc(processorPrefix+"core_count", "Number of cores of processor", "host", "socket", "model")
	processorThreadsDesc = newDesc(processorPrefix+"thread_count", "Number of threads of processor", "host", "socket", "model")
	processorHealthyDesc = newDesc(processorPrefix+"healthy", "Health status of processor", "host", "socket", "model")
)

// Processor is a processor resource
type Processor struct {
	ID           string         `json:"Id"`
	Socket       string         `json:"Socket"`
	Model        string         `json:"Model"`
	TotalCores   float64        `json:"TotalCores"`
	TotalThreads float64        `json:"TotalThreads"`
	Status       redfish.Status `json:"Status"`
}

func collectProcessors(ctx context.Context, s *scrape) error {
	path := s.systemLink(func(sys *System) string { return sys.Processors.ID }, "Processors")
	if path == "" {
		return nil
	}

	procs := redfish.Collection{}
	if err := s.cl.Get(ctx, path, &procs); err != nil {
		return fmt.Errorf("could not get processor summary: %w", err)
	}

	s.emit(processorCountDesc, float64(len(procs.Members)), s.host)

	return forEach(ctx, procs.Members, func(ctx context.Context, l redfish.Link) error {
		pr := Processor{}
		if err := s.cl.Get(ctx, l.ID, &pr); err != nil {
			return fmt.Errorf("could not get processor information from %s: %w", l.ID, err)
		}

		socket := pr.Socket
		if socket == "" {
			// e.g. GPUs listed as processors by iLO 6 have no socket
			socket = pr.ID
		}

		labels := []string{s.host, socket, strings.Trim(pr.Model, " ")}
		s.emit(processorCoresDesc, pr.TotalCores, labels...)
		s.emit(processorThreadsDesc, pr.TotalThreads, labels...)
		s.emit(processorHealthyDesc, pr.Status.HealthValue(), labels...)
		return nil
	})
}
