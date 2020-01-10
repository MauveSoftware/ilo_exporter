package storage

import "github.com/MauveSoftware/ilo4_exporter/common"

type Drives struct {
	Total uint32 `json:"Total"`
	Links struct {
		Members []common.Member `json:"Member"`
	} `json:"links"`
}
