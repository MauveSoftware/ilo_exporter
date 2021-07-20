package memory

type Memory struct {
	MemorySummary struct {
		Status struct {
			HealthRollUp string `json:"HealthRollUp"`
		} `json:"Status"`
		TotalSystemMemoryGiB uint64 `json:"TotalSystemMemoryGiB"`
	} `json:"MemorySummary"`
}
