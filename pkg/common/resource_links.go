package common

type ResourceLinks struct {
	Links struct {
		Members []struct {
			Href string `json:"href"`
		} `json:"Member"`
	} `json:"links"`
}