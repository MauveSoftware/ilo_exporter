// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package collector

import (
	"context"
	"fmt"
)

var managerInfoDesc = newDesc(prefix+"manager_info", "System Info", "host", "firmware_version")

// Manager is a manager (iLO) resource
type Manager struct {
	FirmwareVersion string `json:"FirmwareVersion"`
}

func collectManager(ctx context.Context, s *scrape) error {
	m := Manager{}
	if err := s.cl.Get(ctx, s.managerPath, &m); err != nil {
		return fmt.Errorf("could not get manager: %w", err)
	}

	s.emit(managerInfoDesc, 1, s.host, m.FirmwareVersion)
	return nil
}
