package thermal

import (
	"github.com/MauveSoftware/ilo4_exporter/common"
)

type Temperature struct {
	Name                   string        `json:"Name"`
	ReadingCelsius         float64       `json:"ReadingCelsius"`
	UpperThresholdCritical float64       `json:"UpperThresholdCritical"`
	UpperThresholdFatal    float64       `json:"UpperThresholdFatal"`
	Status                 common.Status `json:"Status"`
}
