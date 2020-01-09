package processor

import (
	"github.com/MauveSoftware/ilo4_exporter/common"
)

type Processors struct {
	Count float64 `json:"Count"`
	Links struct {
		Members []common.Member `json:"Member"`
	} `json:"links"`
}
