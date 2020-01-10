package storage

import "github.com/MauveSoftware/ilo4_exporter/common"

type LogicalDrive struct {
	LogicalDriveName string        `json:"LogicalDriveName"`
	Raid             string        `json:"Raid"`
	CapacityMiB      uint64        `json:"CapacityMiB"`
	Status           common.Status `json:"Status"`
}
