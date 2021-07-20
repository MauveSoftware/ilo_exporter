package system

type System struct {
	PowerState string `json:"PowerState"`
}

func (s *System) PowerUpValue() float64 {
	if s.PowerState == "On" {
		return 1
	}

	return 0
}
