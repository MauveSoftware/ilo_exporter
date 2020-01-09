package power

import (
	"github.com/MauveSoftware/ilo4_exporter/common"
)

type PowerSupply struct {
	SerialNumber string        `json:"SerialNumber"`
	Status       common.Status `json:"Status"`
}
