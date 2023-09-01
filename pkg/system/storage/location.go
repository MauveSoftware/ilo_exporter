// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

package storage

import "encoding/json"

type Location string

type LocationInfo struct {
	Info string `json:"Info"`
}

func (f *Location) UnmarshalJSON(data []byte) error {
	var infos []LocationInfo
	json.Unmarshal(data, &infos)

	if len(infos) > 0 {
		*f = Location(infos[0].Info)
		return nil
	}

	var str string
	json.Unmarshal(data, &str)
	*f = Location(str)
	return nil
}
