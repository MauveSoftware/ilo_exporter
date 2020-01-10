package storage

import "github.com/MauveSoftware/ilo4_exporter/common"

type ArrayControllers struct {
	Links struct {
		Members []common.Member `json:"Member"`
	} `json:"links"`
}
