package source

import (
	"strconv"
)

func IsEnabled(annotations map[string]string, prefix string) bool {
	return annotations[prefix+"/enabled"] == "true"
}

func ParseAnnotations(annotations map[string]string, prefix string) DashboardEntry {
	priority, _ := strconv.Atoi(annotations[prefix+"/priority"])
	return DashboardEntry{
		Name:                 annotations[prefix+"/name"],
		URL:                  annotations[prefix+"/url"],
		IconURL:              annotations[prefix+"/icon"],
		Description:          annotations[prefix+"/description"],
		PingURL:              annotations[prefix+"/ping-url"],
		Group:                annotations[prefix+"/group"],
		Priority:             priority,
		IntegrationType:      annotations[prefix+"/integration-type"],
		IntegrationURL:       annotations[prefix+"/integration-url"],
		IntegrationSecret:    annotations[prefix+"/integration-secret"],
		IntegrationSecretKey: annotations[prefix+"/integration-secret-key"],
		Widget:               annotations[prefix+"/widget"],
	}
}
