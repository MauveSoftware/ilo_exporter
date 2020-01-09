package memory

import "github.com/MauveSoftware/ilo4_exporter/common"

type Memory struct {
	Status struct {
		HealthRollUp string `json:"HealthRollUp"`
	} `json:"Status"`
	TotalSystemMemoryGiB uint64 `json:"TotalSystemMemoryGiB"`
	Links                struct {
		Members []common.Member `json:"Member"`
	} `json:"links"`
}
