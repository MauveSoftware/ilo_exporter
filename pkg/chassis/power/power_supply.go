package power

import (
	"github.com/MauveSoftware/ilo4_exporter/pkg/common"
)

type PowerSupply struct {
	SerialNumber string        `json:"SerialNumber"`
	Status       common.Status `json:"Status"`
}
