package processor

import "github.com/MauveSoftware/ilo4_exporter/common"

type Processor struct {
	Socket       string        `json:"Socket"`
	Model        string        `json:"Model"`
	TotalCores   float64       `json:"TotalCores"`
	TotalThreads float64       `json:"TotalThreads"`
	Status       common.Status `json:"Status"`
}
