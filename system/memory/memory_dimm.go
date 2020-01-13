package memory
type MemoryDIMM struct {
	Name       string `json:"Name"`
	DIMMStatus string `json:"DIMMStatus"`
	SizeMB     uint64 `json:"SizeMB"`
}
