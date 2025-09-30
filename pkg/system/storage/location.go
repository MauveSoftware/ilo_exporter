// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import (
	"encoding/json"
)

type Location string

type LocationInfo struct {
	Info string `json:"Info"`
}

// PhysicalLocationInfo is returned by ILO5
type PhysicalLocationInfo struct {
	PartLocation PartLocationInfo `json:"PartLocation"`
}

type PartLocationInfo struct {
	ServiceLabel         string `json:"ServiceLabel"`         // Slot=12:Port=1:Box=1:Bay=1",
	LocationType         string `json:"LocationType"`         // "Bay",
	LocationOrdinalValue uint   `json:"LocationOrdinalValue"` // 1
}

func (f *Location) UnmarshalJSON(data []byte) error {
	var infos []LocationInfo
	_ = json.Unmarshal(data, &infos)

	if len(infos) > 0 {
		*f = Location(infos[0].Info)
		return nil
	}

	var info PhysicalLocationInfo
	_ = json.Unmarshal(data, &info)

	if info.PartLocation.ServiceLabel != "" {
		*f = Location(info.PartLocation.ServiceLabel)
		return nil
	}

	var str string
	_ = json.Unmarshal(data, &str)
	*f = Location(str)
	return nil
}
