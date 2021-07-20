package thermal

type Thermal struct {
	Temperatures []Temperature `json:"Temperatures"`
	Fans         []Fan         `json:"Fans"`
}
